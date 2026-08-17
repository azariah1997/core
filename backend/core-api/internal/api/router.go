package api

import (
	"context"
	"net/http"
	"time"

	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

func New(cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"status": "ok", "service": "core-api"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		checks := []health.Result{
			health.TCP(ctx, health.Check{Name: "postgres", Address: cfg.PostgresDSN}),
			health.TCP(ctx, health.Check{Name: "valkey", Address: cfg.RedisAddr}),
			health.TCP(ctx, health.Check{Name: "kafka", Address: cfg.KafkaBrokers[0]}),
		}
		ok := true
		for _, c := range checks {
			if !c.OK {
				ok = false
			}
		}
		status := 200
		if !ok {
			status = 503
		}
		httpx.JSON(w, status, map[string]any{"ready": ok, "dependencies": checks})
	})
	mux.HandleFunc("GET /v1/platform", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"name": cfg.PlatformName, "environment": cfg.Env, "apiVersion": "v1"})
	})
	mux.HandleFunc("GET /v1/users/me", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"id": "local-demo-user", "displayName": "Local Developer", "locale": "en-GB", "source": "core-user-domain"})
	})
	mux.HandleFunc("POST /v1/data/query", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusNotImplemented, map[string]any{"error": "generic database queries are intentionally disabled", "guidance": "use domain/query contracts so storage can evolve independently"})
	})
	return requestID(mux)
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = time.Now().UTC().Format("20060102T150405.000000000")
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}
