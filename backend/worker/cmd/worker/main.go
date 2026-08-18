package main

import (
	"context"
	"net/http"
	"os"
	"time"

	temporalclient "go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"

	"github.com/example/core-platform/backend/worker/internal/indexer"
	"github.com/example/core-platform/backend/worker/internal/jobrunner"
	"github.com/example/core-platform/backend/worker/internal/jobrunner/handlers"
	"github.com/example/core-platform/backend/worker/internal/workflows"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/runx"
	"github.com/example/core-platform/packages/go/platformkit/searchidx"
	"github.com/example/core-platform/packages/go/platformkit/workflowkit"
)

const serviceName = "worker"

// indexerPollInterval trades a little indexing latency (new documents
// take up to this long to become searchable) for not hammering Postgres
// with a tight poll loop - fine for a local/single-replica indexer; a
// LISTEN/NOTIFY or Kafka-driven trigger would remove the latency
// entirely, deferred until a real need for lower latency shows up.
const indexerPollInterval = 2 * time.Second

// jobPollInterval is shorter than the search indexer's - jobs (a
// "delayed" webhook, for instance) are more latency-sensitive than
// search freshness, and claiming is cheap (a single indexed query) even
// when there's nothing due.
const jobPollInterval = 1 * time.Second

func main() {
	cfg := config.Load()
	logger := logging.New(serviceName, cfg.Env)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	shutdownTracing, err := otelx.Init(ctx, otelx.Config{
		ServiceName: serviceName,
		Environment: cfg.Env,
		Endpoint:    cfg.OtelEndpoint,
	})
	if err != nil {
		logger.Error("failed to initialise tracing", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	pool, err := pg.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	searchProvider, err := searchidx.NewOpenSearchProvider(ctx, searchidx.OpenSearchConfig{
		Addresses: []string{cfg.OpenSearchURL}, Index: searchidx.DefaultIndex,
	})
	if err != nil {
		logger.Error("failed to initialise opensearch provider", "error", err)
		os.Exit(1)
	}
	docIndexer := indexer.New(pool, searchProvider, logger)

	jobRunner := jobrunner.New(pool, logger)
	jobRunner.Register("echo", handlers.Echo(logger))
	jobRunner.Register("webhook", handlers.Webhook(nil))

	temporalConn, err := temporalclient.Dial(temporalclient.Options{HostPort: cfg.TemporalAddr})
	if err != nil {
		logger.Error("failed to connect to temporal", "error", err)
		os.Exit(1)
	}
	defer temporalConn.Close()
	temporalWorker := temporalworker.New(temporalConn, workflowkit.TaskQueue, temporalworker.Options{})
	workflows.Register(temporalWorker)
	if err := temporalWorker.Start(); err != nil {
		logger.Error("failed to start temporal worker", "error", err)
		os.Exit(1)
	}
	defer temporalWorker.Stop()

	dependencyChecks := func(ctx context.Context) []health.Result {
		return []health.Result{
			health.TCP(ctx, health.Check{Name: "kafka", Address: cfg.KafkaBrokers[0]}),
			health.TCP(ctx, health.Check{Name: "postgres", Address: cfg.PostgresDSN}),
			health.TCP(ctx, health.Check{Name: "temporal", Address: cfg.TemporalAddr}),
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health.Live(serviceName))
	mux.HandleFunc("GET /readyz", health.Ready(dependencyChecks))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, dependencyChecks))

	handler := otelx.Wrap(serviceName, correlation.Middleware(mux))
	srv := &http.Server{Addr: cfg.WorkerAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(indexerPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				processed, err := docIndexer.PollOnce(ctx)
				if err != nil {
					logger.Error("search indexer poll failed", "error", err)
					continue
				}
				if processed > 0 {
					logger.Info("search indexer processed events", "count", processed)
				}
			case <-stop:
				return
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(jobPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				processed, err := jobRunner.PollOnce(ctx)
				if err != nil {
					logger.Error("job runner poll failed", "error", err)
					continue
				}
				if processed > 0 {
					logger.Info("job runner processed jobs", "count", processed)
				}
			case <-stop:
				return
			}
		}
	}()

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
