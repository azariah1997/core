package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/api"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	pulseconnectionspg "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
	pulseprofilepg "github.com/example/core-platform/apps/pulse/api/internal/pulseprofile/postgres"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/runx"
)

const serviceName = "pulse-api"

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := logging.New(serviceName, env("PLATFORM_ENV", "local"))

	ctx := context.Background()
	shutdownTracing, err := otelx.Init(ctx, otelx.Config{
		ServiceName: serviceName,
		Environment: env("PLATFORM_ENV", "local"),
		Endpoint:    os.Getenv("OTEL_ENDPOINT"),
	})
	if err != nil {
		logger.Error("failed to initialise tracing", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	// Pulse's own Postgres database - never Core's, per
	// apps/pulse/docs/ARCHITECTURE_AUDIT.md's data ownership section.
	dsn := env("PULSE_POSTGRES_DSN", "postgres://core:core@localhost:5432/pulse?sslmode=disable")
	pool, err := pg.Connect(ctx, dsn)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	defer pg.ReportStats(ctx, serviceName, pool, 10*time.Second)()

	profileSvc := pulseprofile.NewService(pulseprofilepg.New(pool))
	connectionsSvc := pulseconnections.NewService(pulseconnectionspg.New(pool))

	cfg := api.Config{
		Version:    config.Load().Version,
		CoreAPIURL: env("CORE_API_URL", "http://localhost:8080"),
		PulseAppID: env("PULSE_APP_ID", ""),
	}
	if cfg.PulseAppID == "" {
		logger.Error("PULSE_APP_ID is required - register Pulse via POST /v1/apps first (see apps/pulse/api/README.md)")
		os.Exit(1)
	}
	handler := otelx.Wrap(serviceName, api.New(cfg, pool, profileSvc, connectionsSvc))

	addr := env("PULSE_HTTP_ADDR", ":8096")
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
