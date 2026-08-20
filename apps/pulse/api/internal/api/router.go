// Package api assembles the Pulse Product API's HTTP surface -
// health/metrics plus every Pulse domain module's routes, each wired
// with pulseauth.RequireUser as its auth middleware.
package api

import (
	"context"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "pulse-api"

type Config struct {
	Version    string
	CoreAPIURL string
}

func New(cfg Config, pool *pgxpool.Pool, profileSvc *pulseprofile.Service) http.Handler {
	mux := http.NewServeMux()

	checks := func(ctx context.Context) []health.Result {
		err := pool.Ping(ctx)
		return []health.Result{{Name: "postgres", OK: err == nil, Error: errString(err)}}
	}
	mux.HandleFunc("GET /livez", health.Live(serviceName, cfg.Version))
	mux.HandleFunc("GET /readyz", health.Ready(checks))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, cfg.Version, checks))
	mux.Handle("GET /metrics", metrics.Handler())

	requireUser := pulseauth.RequireUser(cfg.CoreAPIURL)
	pulseprofile.RegisterRoutes(mux, profileSvc, requireUser)

	return metrics.Middleware(serviceName, mux, corsMiddleware(correlation.Middleware(mux)))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
