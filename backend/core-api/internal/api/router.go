package api

import (
	"context"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/backend/core-api/internal/audit"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/features"
	"github.com/example/core-platform/backend/core-api/internal/files"
	"github.com/example/core-platform/backend/core-api/internal/groups"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/jobs"
	"github.com/example/core-platform/backend/core-api/internal/messaging"
	"github.com/example/core-platform/backend/core-api/internal/notifications"
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const serviceName = "core-api"

func New(cfg config.Config, apps *applications.Service, identitySvc *identity.Service, usersSvc *users.Service, devicesSvc *devices.Service, authzSvc *authz.Service, tenantsSvc *tenants.Service, relationshipsSvc *relationships.Service, groupsSvc *groups.Service, messagingSvc *messaging.Service, notificationsSvc *notifications.Service, filesSvc *files.Service, searchSvc *search.Service, jobsSvc *jobs.Service, workflowsSvc *workflows.Service, featuresSvc *features.Service, remoteConfigSvc *remoteconfig.Service, auditSvc *audit.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /livez", health.Live(serviceName))
	mux.HandleFunc("GET /readyz", health.Ready(dependencyChecks(cfg)))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, dependencyChecks(cfg)))

	applications.RegisterRoutes(mux, apps)
	identity.RegisterRoutes(mux, identitySvc)
	users.RegisterRoutes(mux, usersSvc, requireUser(identitySvc, usersSvc), newProfileAccessChecker(authzSvc))
	devices.RegisterRoutes(mux, devicesSvc, requireUser(identitySvc, usersSvc))
	tenants.RegisterRoutes(mux, tenantsSvc, requireUser(identitySvc, usersSvc))
	relationships.RegisterRoutes(mux, relationshipsSvc, requireUser(identitySvc, usersSvc))
	groups.RegisterRoutes(mux, groupsSvc, requireUser(identitySvc, usersSvc))
	messaging.RegisterRoutes(mux, messagingSvc, requireUser(identitySvc, usersSvc))
	notifications.RegisterRoutes(mux, notificationsSvc, requireUser(identitySvc, usersSvc))
	files.RegisterRoutes(mux, filesSvc, requireUser(identitySvc, usersSvc))
	search.RegisterRoutes(mux, searchSvc, requireUser(identitySvc, usersSvc))
	jobs.RegisterRoutes(mux, jobsSvc, requireUser(identitySvc, usersSvc))
	workflows.RegisterRoutes(mux, workflowsSvc, requireUser(identitySvc, usersSvc))
	features.RegisterRoutes(mux, featuresSvc, requireUser(identitySvc, usersSvc))
	remoteconfig.RegisterRoutes(mux, remoteConfigSvc, requireUser(identitySvc, usersSvc))
	audit.RegisterRoutes(mux, auditSvc, requireUser(identitySvc, usersSvc))

	mux.HandleFunc("GET /v1/platform", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"name": cfg.PlatformName, "environment": cfg.Env, "apiVersion": "v1"})
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
			health.TCP(ctx, health.Check{Name: "temporal", Address: cfg.TemporalAddr}),
		}
	}
}
