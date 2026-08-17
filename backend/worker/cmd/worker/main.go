package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/runx"
)

const serviceName = "worker"

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

	dependencyChecks := func(ctx context.Context) []health.Result {
		return []health.Result{health.TCP(ctx, health.Check{Name: "kafka", Address: cfg.KafkaBrokers[0]})}
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
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logger.Info("worker heartbeat", "brokers", cfg.KafkaBrokers)
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
