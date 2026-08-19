package api

import (
	"context"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
	"github.com/example/core-platform/backend/core-api/internal/analytics"
	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/backend/core-api/internal/audit"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/billing"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/features"
	"github.com/example/core-platform/backend/core-api/internal/files"
	"github.com/example/core-platform/backend/core-api/internal/groups"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/jobs"
	"github.com/example/core-platform/backend/core-api/internal/messaging"
	"github.com/example/core-platform/backend/core-api/internal/notifications"
	"github.com/example/core-platform/backend/core-api/internal/privacy"
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
	"github.com/example/core-platform/packages/go/platformkit/metrics"
)

const serviceName = "core-api"

func New(cfg config.Config, apps *applications.Service, identitySvc *identity.Service, usersSvc *users.Service, devicesSvc *devices.Service, authzSvc *authz.Service, tenantsSvc *tenants.Service, relationshipsSvc *relationships.Service, groupsSvc *groups.Service, messagingSvc *messaging.Service, notificationsSvc *notifications.Service, filesSvc *files.Service, searchSvc *search.Service, jobsSvc *jobs.Service, workflowsSvc *workflows.Service, featuresSvc *features.Service, remoteConfigSvc *remoteconfig.Service, auditSvc *audit.Service, privacySvc *privacy.Service, trustSafetySvc *trustsafety.Service, billingSvc *billing.Service, analyticsSvc *analytics.Service, aiGatewaySvc *aigateway.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /livez", health.Live(serviceName, cfg.Version))
	mux.HandleFunc("GET /readyz", health.Ready(dependencyChecks(cfg)))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, cfg.Version, dependencyChecks(cfg)))
	mux.Handle("GET /metrics", metrics.Handler())

	applications.RegisterRoutes(mux, apps)
	identity.RegisterRoutes(mux, identitySvc)
	// users and trustsafety deliberately use plain requireUser, not
	// requireActive - see requireActive's own doc comment (session.go)
	// for why a restricted user must never lose access to either.
	users.RegisterRoutes(mux, usersSvc, requireUser(identitySvc, usersSvc), newProfileAccessChecker(authzSvc))
	trustsafety.RegisterRoutes(mux, trustSafetySvc, requireUser(identitySvc, usersSvc))
	devices.RegisterRoutes(mux, devicesSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	tenants.RegisterRoutes(mux, tenantsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	relationships.RegisterRoutes(mux, relationshipsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	groups.RegisterRoutes(mux, groupsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	messaging.RegisterRoutes(mux, messagingSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	notifications.RegisterRoutes(mux, notificationsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	files.RegisterRoutes(mux, filesSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	search.RegisterRoutes(mux, searchSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	jobs.RegisterRoutes(mux, jobsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	workflows.RegisterRoutes(mux, workflowsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	features.RegisterRoutes(mux, featuresSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	remoteconfig.RegisterRoutes(mux, remoteConfigSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	audit.RegisterRoutes(mux, auditSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	privacy.RegisterRoutes(mux, privacySvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	billing.RegisterRoutes(mux, billingSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	// The webhook receiver authenticates via Stripe's own signature, not
	// a platform Bearer token - registered with no requireUser/
	// requireActive wrapper at all, the one deliberate exception. See
	// billing/http.go's RegisterWebhookRoute and billing/README.md.
	billing.RegisterWebhookRoute(mux, billingSvc)
	analytics.RegisterRoutes(mux, analyticsSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	// Tracking is this platform's one deliberately open, unauthenticated
	// write endpoint - no requireUser/requireActive wrapper at all,
	// protected by IP-based rate limiting instead. See
	// analytics/http.go's RegisterTrackRoute and analytics/README.md.
	analytics.RegisterTrackRoute(mux, analyticsSvc)
	aigateway.RegisterRoutes(mux, aiGatewaySvc, requireActive(identitySvc, usersSvc, trustSafetySvc))
	// authz's first-ever HTTP surface (Phase 25) - role management is
	// gated platform.admin-only inside the handlers themselves.
	authz.RegisterRoutes(mux, authzSvc, requireActive(identitySvc, usersSvc, trustSafetySvc))

	mux.HandleFunc("GET /v1/platform", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"name": cfg.PlatformName, "environment": cfg.Env, "apiVersion": "v1", "version": cfg.Version})
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

	return metrics.Middleware(serviceName, mux, corsMiddleware(correlation.Middleware(mux)))
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
