package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/apps/pulse/api/internal/api"
	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	bondpg "github.com/example/core-platform/apps/pulse/api/internal/bond/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
	livetouchcore "github.com/example/core-platform/apps/pulse/api/internal/livetouch/core"
	livetouchpg "github.com/example/core-platform/apps/pulse/api/internal/livetouch/postgres"
	livetouchpulsemodules "github.com/example/core-platform/apps/pulse/api/internal/livetouch/pulsemodules"
	"github.com/example/core-platform/apps/pulse/api/internal/moments"
	momentspg "github.com/example/core-platform/apps/pulse/api/internal/moments/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/mood"
	moodcore "github.com/example/core-platform/apps/pulse/api/internal/mood/core"
	moodpg "github.com/example/core-platform/apps/pulse/api/internal/mood/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/mood/pulsemodules"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	pulseconnectionspg "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	pulseinteractionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/core"
	pulseinteractionspg "github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
	pulseprefspg "github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
	pulseprofilepg "github.com/example/core-platform/apps/pulse/api/internal/pulseprofile/postgres"
	"github.com/example/core-platform/apps/pulse/api/internal/signals"
	signalspg "github.com/example/core-platform/apps/pulse/api/internal/signals/postgres"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/metrics"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/ratelimit"
	"github.com/example/core-platform/packages/go/platformkit/rtbus"
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

	// Pulse's first Redis (Valkey) dependency - real-time delivery
	// (rtbus, the shared contract realtime-gateway's hub already
	// subscribes to) and rate limiting both need it. Same shared
	// instance Core's own services use, addressed independently since
	// pulse-api is its own deployable.
	redisAddr := env("PULSE_REDIS_ADDR", "localhost:6379")
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	metrics.InstrumentRedis(redisClient)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis/valkey", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	cfg := api.Config{
		Version:        config.Load().Version,
		CoreAPIURL:     env("CORE_API_URL", "http://localhost:8080"),
		RealtimeAPIURL: env("REALTIME_API_URL", "http://localhost:8090"),
		PulseAppID:     env("PULSE_APP_ID", ""),
	}
	if cfg.PulseAppID == "" {
		logger.Error("PULSE_APP_ID is required - register Pulse via POST /v1/apps first (see apps/pulse/api/README.md)")
		os.Exit(1)
	}

	profileSvc := pulseprofile.NewService(pulseprofilepg.New(pool))
	connectionsSvc := pulseconnections.NewService(pulseconnectionspg.New(pool))
	bondSvc := bond.NewService(bondpg.New(pool))

	realtimeAdapter := pulseinteractionscore.NewRealtimeAdapter(rtbus.NewPublisher(redisClient))
	analyticsAdapter := pulseinteractionscore.NewAnalyticsAdapter(cfg.CoreAPIURL, cfg.PulseAppID)
	interactionsSvc := pulseinteractions.NewService(pulseinteractionspg.New(pool), realtimeAdapter, analyticsAdapter, ratelimit.New(redisClient))

	moodSvc := mood.NewService(moodpg.New(pool), pulsemodules.NewBondAdapter(bondSvc), moodcore.NewAnalyticsAdapter(cfg.CoreAPIURL, cfg.PulseAppID))

	liveTouchRealtime := livetouchcore.NewRealtimeAdapter(rtbus.NewPublisher(redisClient))
	liveTouchAnalytics := livetouchcore.NewAnalyticsAdapter(cfg.CoreAPIURL, cfg.PulseAppID)
	liveTouchSvc := livetouch.NewService(livetouchpg.New(pool), livetouchpulsemodules.NewBondAdapter(bondSvc), liveTouchRealtime, liveTouchAnalytics, ratelimit.New(redisClient))

	signalsSvc := signals.NewService(signalspg.New(pool))

	momentsSvc := moments.NewService(momentspg.New(pool))

	prefsSvc := pulseprefs.NewService(pulseprefspg.New(pool))

	handler := otelx.Wrap(serviceName, api.New(cfg, pool, profileSvc, connectionsSvc, bondSvc, interactionsSvc, moodSvc, liveTouchSvc, signalsSvc, momentsSvc, prefsSvc))

	addr := env("PULSE_HTTP_ADDR", ":8096")
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
