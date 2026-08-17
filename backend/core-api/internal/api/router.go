package api

import (
	"context"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const serviceName = "core-api"

func New(cfg config.Config, apps *applications.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /livez", health.Live(serviceName))
	mux.HandleFunc("GET /readyz", health.Ready(dependencyChecks(cfg)))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, dependencyChecks(cfg)))

	applications.RegisterRoutes(mux, apps)

	mux.HandleFunc("GET /v1/platform", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"name": cfg.PlatformName, "environment": cfg.Env, "apiVersion": "v1"})
	})
	mux.HandleFunc("GET /v1/users/me", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"id": "local-demo-user", "displayName": "Local Developer", "locale": "en-GB", "source": "core-user-domain"})
	})
	mux.HandleFunc("POST /v1/data/query", func(w http.ResponseWriter, r *http.Request) {
		apperr.Write(w, r, apperr.New(apperr.CodeNotImplemented,
			"generic database queries are intentionally disabled; use domain/query contracts so storage can evolve independently"))
	})

	// Unmatched routes return the platform's standard error envelope
	// instead of Go's default plaintext 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "resource not found"))
	})

	return correlation.Middleware(mux)
}

func dependencyChecks(cfg config.Config) health.Checks {
	return func(ctx context.Context) []health.Result {
		return []health.Result{
			health.TCP(ctx, health.Check{Name: "postgres", Address: cfg.PostgresDSN}),
			health.TCP(ctx, health.Check{Name: "valkey", Address: cfg.RedisAddr}),
			health.TCP(ctx, health.Check{Name: "kafka", Address: cfg.KafkaBrokers[0]}),
		}
	}
}
