// Package api assembles the Pulse Product API's HTTP surface -
// health/metrics plus every Pulse domain module's routes, each wired
// with pulseauth.RequireUser as its auth middleware.
package api

import (
	"context"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	bondcore "github.com/example/core-platform/apps/pulse/api/internal/bond/core"
	"github.com/example/core-platform/apps/pulse/api/internal/mood"
	"github.com/example/core-platform/apps/pulse/api/internal/mood/pulsemodules"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	pulseconnectionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	pulseinteractionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

const serviceName = "pulse-api"

type Config struct {
	Version    string
	CoreAPIURL string
	// RealtimeAPIURL is realtime-gateway's own base URL - pulse-
	// interactions' presence check (Phase 5) is the one thing this
	// service calls there directly.
	RealtimeAPIURL string
	// PulseAppID is Pulse's own real Core Application ID (registered
	// once via POST /v1/apps during Phase 1) - every Core relationships
	// call pulse-connections makes needs it.
	PulseAppID string
}

func New(cfg Config, pool *pgxpool.Pool, profileSvc *pulseprofile.Service, connectionsSvc *pulseconnections.Service, bondSvc *bond.Service, interactionsSvc *pulseinteractions.Service, moodSvc *mood.Service) http.Handler {
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

	newConnectionsCore := func(client *coresdk.Client) pulseconnections.CoreRelationships {
		return pulseconnectionscore.New(client, cfg.PulseAppID)
	}
	pulseconnections.RegisterRoutes(mux, connectionsSvc, newConnectionsCore, requireUser)

	newBondCore := func(client *coresdk.Client) bond.CoreRelationships {
		return bondcore.New(client, cfg.PulseAppID)
	}
	bond.RegisterRoutes(mux, bondSvc, newBondCore, requireUser)

	newInteractionsCore := func(client *coresdk.Client) pulseinteractions.CoreRelationships {
		return pulseinteractionscore.NewRelationshipsAdapter(client, cfg.PulseAppID)
	}
	newPresence := func(token string) pulseinteractions.Presence {
		return pulseinteractionscore.NewPresenceAdapter(cfg.RealtimeAPIURL, token)
	}
	newNotifier := func(client *coresdk.Client) pulseinteractions.Notifier {
		return pulseinteractionscore.NewNotifierAdapter(client, cfg.PulseAppID)
	}
	pulseinteractions.RegisterRoutes(mux, interactionsSvc, newInteractionsCore, newPresence, newNotifier, requireUser)

	newMoodConnections := func(client *coresdk.Client) mood.Connections {
		return pulsemodules.NewConnectionsAdapter(connectionsSvc, newConnectionsCore(client))
	}
	mood.RegisterRoutes(mux, moodSvc, newMoodConnections, requireUser)

	return metrics.Middleware(serviceName, mux, corsMiddleware(correlation.Middleware(mux)))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
