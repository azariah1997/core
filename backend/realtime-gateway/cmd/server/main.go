package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/realtime-gateway/internal/hub"
	"github.com/example/core-platform/backend/realtime-gateway/internal/identity"
	"github.com/example/core-platform/backend/realtime-gateway/internal/presence"
	"github.com/example/core-platform/backend/realtime-gateway/internal/ws"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/jwtverify"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/runx"
)

const serviceName = "realtime-gateway"

// presenceTTL bounds how stale presence can get before a connection that
// died without a clean close (crash, network partition) ages out - never
// stored in Postgres, matching "critical realtime data should remain
// ephemeral".
const presenceTTL = 45 * time.Second

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

	// Read-only: realtime-gateway never writes to Postgres, only resolves
	// an authenticated identity to the platform user ID everything else
	// in the system already addresses users by.
	pool, err := pg.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis/valkey", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	verifier, err := jwtverify.New(ctx, jwtverify.Config{Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience})
	if err != nil {
		logger.Error("failed to initialise jwt verifier", "error", err)
		os.Exit(1)
	}
	resolver := identity.NewResolver(pool)
	presenceTracker := presence.New(redisClient, presenceTTL)
	realtimeHub := hub.New(redisClient, logger)

	hubCtx, cancelHub := context.WithCancel(ctx)
	defer cancelHub()
	go realtimeHub.Run(hubCtx)

	dependencyChecks := func(ctx context.Context) []health.Result {
		return []health.Result{health.TCP(ctx, health.Check{Name: "redis", Address: cfg.RedisAddr})}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health.Live(serviceName))
	mux.HandleFunc("GET /readyz", health.Ready(dependencyChecks))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, dependencyChecks))
	mux.Handle("GET /ws", wsAuthMiddleware(verifier, resolver)(ws.NewHandler(realtimeHub, presenceTracker, logger)))

	handler := otelx.Wrap(serviceName, correlation.Middleware(mux))
	srv := &http.Server{Addr: cfg.RealtimeAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
