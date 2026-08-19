package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/core-api/internal/analytics"
	analyticspg "github.com/example/core-platform/backend/core-api/internal/analytics/postgres"
	"github.com/example/core-platform/backend/core-api/internal/api"
	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationspg "github.com/example/core-platform/backend/core-api/internal/applications/postgres"
	"github.com/example/core-platform/backend/core-api/internal/audit"
	auditpg "github.com/example/core-platform/backend/core-api/internal/audit/postgres"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/authz/openfga"
	authzpg "github.com/example/core-platform/backend/core-api/internal/authz/postgres"
	"github.com/example/core-platform/backend/core-api/internal/billing"
	billingpg "github.com/example/core-platform/backend/core-api/internal/billing/postgres"
	"github.com/example/core-platform/backend/core-api/internal/billing/stripe"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	devicespg "github.com/example/core-platform/backend/core-api/internal/devices/postgres"
	"github.com/example/core-platform/backend/core-api/internal/features"
	featurespg "github.com/example/core-platform/backend/core-api/internal/features/postgres"
	"github.com/example/core-platform/backend/core-api/internal/files"
	filespg "github.com/example/core-platform/backend/core-api/internal/files/postgres"
	filess3 "github.com/example/core-platform/backend/core-api/internal/files/s3"
	"github.com/example/core-platform/backend/core-api/internal/groups"
	groupspg "github.com/example/core-platform/backend/core-api/internal/groups/postgres"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/identity/keycloak"
	identitypg "github.com/example/core-platform/backend/core-api/internal/identity/postgres"
	"github.com/example/core-platform/backend/core-api/internal/jobs"
	jobspg "github.com/example/core-platform/backend/core-api/internal/jobs/postgres"
	"github.com/example/core-platform/backend/core-api/internal/messaging"
	messagingpg "github.com/example/core-platform/backend/core-api/internal/messaging/postgres"
	"github.com/example/core-platform/backend/core-api/internal/notifications"
	notificationspg "github.com/example/core-platform/backend/core-api/internal/notifications/postgres"
	"github.com/example/core-platform/backend/core-api/internal/notifications/senders"
	"github.com/example/core-platform/backend/core-api/internal/privacy"
	privacypg "github.com/example/core-platform/backend/core-api/internal/privacy/postgres"
	privacyworkflow "github.com/example/core-platform/backend/core-api/internal/privacy/workflow"
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	relationshipspg "github.com/example/core-platform/backend/core-api/internal/relationships/postgres"
	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	remoteconfigpg "github.com/example/core-platform/backend/core-api/internal/remoteconfig/postgres"
	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	tenantspg "github.com/example/core-platform/backend/core-api/internal/tenants/postgres"
	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
	trustsafetypg "github.com/example/core-platform/backend/core-api/internal/trustsafety/postgres"
	"github.com/example/core-platform/backend/core-api/internal/users"
	userspg "github.com/example/core-platform/backend/core-api/internal/users/postgres"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
	workflowspg "github.com/example/core-platform/backend/core-api/internal/workflows/postgres"
	workflowstemporal "github.com/example/core-platform/backend/core-api/internal/workflows/temporal"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/ratelimit"
	"github.com/example/core-platform/packages/go/platformkit/rtbus"
	"github.com/example/core-platform/packages/go/platformkit/runx"
	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

const serviceName = "core-api"

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

	pool, err := pg.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// core-api's first Redis client (Valkey was health-checked over TCP
	// only until now): messaging needs it to push new messages to
	// realtime-gateway's hub over the shared rtbus pub/sub contract.
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis/valkey", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	apps := applications.NewService(applicationspg.New(pool))

	keycloakProvider, err := keycloak.New(ctx, keycloak.Config{
		BaseURL:       cfg.KeycloakURL,
		Realm:         cfg.KeycloakRealm,
		Audience:      cfg.JWTAudience,
		AdminUsername: cfg.KeycloakAdminUsername,
		AdminPassword: cfg.KeycloakAdminPassword,
	})
	if err != nil {
		logger.Error("failed to initialise keycloak identity provider", "error", err)
		os.Exit(1)
	}
	identitySvc := identity.NewService("keycloak", keycloakProvider, identitypg.New(pool))
	usersSvc := users.NewService(userspg.New(pool))
	devicesSvc := devices.NewService(devicespg.New(pool))

	openfgaProvider, err := openfga.New(ctx, openfga.Config{APIURL: cfg.OpenFGAURL})
	if err != nil {
		logger.Error("failed to initialise openfga authorization provider", "error", err)
		os.Exit(1)
	}
	// authz.Service needs to record audit events, and audit.Service needs
	// authz.Service as its own AdminChecker - a construction-order cycle
	// broken by wiring the recorder in two phases (see
	// internal/api/audit_adapter.go's doc comment for why this is safe).
	roleChangeAuditRecorder := api.NewRoleChangeAuditRecorder()
	authzSvc := authz.NewService(authzpg.New(pool), openfgaProvider, roleChangeAuditRecorder, logger)
	auditSvc := audit.NewService(auditpg.New(pool), authzSvc)
	roleChangeAuditRecorder.SetAuditService(auditSvc)

	tenantsSvc := tenants.NewService(tenantspg.New(pool))
	relationshipsSvc := relationships.NewService(relationshipspg.New(pool))
	groupsSvc := groups.NewService(groupspg.New(pool))
	messagingSvc := messaging.NewService(messagingpg.New(pool), rtbus.NewPublisher(redisClient), logger)

	notificationSenders := map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelPush:     senders.PushSender{Devices: api.NewDevicePushTokenLookup(devicesSvc), Logger: logger},
		notifications.ChannelEmail:    senders.LogSender{Channel: notifications.ChannelEmail, Logger: logger},
		notifications.ChannelSMS:      senders.LogSender{Channel: notifications.ChannelSMS, Logger: logger},
		notifications.ChannelInApp:    senders.InAppSender{},
		notifications.ChannelRealtime: senders.RealtimeSender{Realtime: rtbus.NewPublisher(redisClient)},
	}
	notificationsSvc := notifications.NewService(notificationspg.New(pool), notificationSenders, authzSvc, logger)

	objectStore, err := filess3.New(ctx, filess3.Config{
		Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		logger.Error("failed to initialise object storage", "error", err)
		os.Exit(1)
	}
	filesSvc := files.NewService(filespg.New(pool), objectStore, authzSvc, files.Config{})

	searchProvider, err := searchidx.NewOpenSearchProvider(ctx, searchidx.OpenSearchConfig{
		Addresses: []string{cfg.OpenSearchURL}, Index: searchidx.DefaultIndex,
	})
	if err != nil {
		logger.Error("failed to initialise opensearch provider", "error", err)
		os.Exit(1)
	}
	searchSvc := search.NewService(searchProvider, authzSvc)

	jobsSvc := jobs.NewService(jobspg.New(pool), authzSvc)

	temporalClient, err := workflowstemporal.New(workflowstemporal.Config{HostPort: cfg.TemporalAddr})
	if err != nil {
		logger.Error("failed to connect to temporal", "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()
	workflowsSvc := workflows.NewService(workflowspg.New(pool), temporalClient, authzSvc)

	featuresSvc := features.NewService(featurespg.New(pool), authzSvc)

	remoteConfigSvc := remoteconfig.NewService(remoteconfigpg.New(pool), authzSvc)

	// Phase 20: privacy.Service reuses temporalClient (WorkflowStarter)
	// and objectStore (ExportStore, now that files/s3.Store has
	// PutObject) directly - both already satisfy the interfaces it
	// needs, no adapter required. Its own worker is a second, separate
	// Temporal worker (see internal/privacy/workflow's doc comment for
	// why) started further down, once every participant is registered.
	privacySvc := privacy.NewService(privacypg.New(pool), authzSvc, temporalClient, objectStore)

	usersPrivacyParticipant := api.NewUsersPrivacyParticipant(usersSvc)
	privacySvc.RegisterExporter("users", usersPrivacyParticipant)
	privacySvc.RegisterDeleter("users", usersPrivacyParticipant)

	devicesPrivacyParticipant := api.NewDevicesPrivacyParticipant(devicesSvc)
	privacySvc.RegisterExporter("devices", devicesPrivacyParticipant)
	privacySvc.RegisterDeleter("devices", devicesPrivacyParticipant)

	filesPrivacyParticipant := api.NewFilesPrivacyParticipant(filesSvc)
	privacySvc.RegisterExporter("files", filesPrivacyParticipant)
	privacySvc.RegisterDeleter("files", filesPrivacyParticipant)

	// audit is Exporter-only, deliberately never registered as a
	// Deleter - see internal/api/privacy_adapters.go's doc comment.
	privacySvc.RegisterExporter("audit", api.NewAuditPrivacyParticipant(auditSvc))

	stopPrivacyWorker, err := privacyworkflow.StartWorker(cfg.TemporalAddr, privacySvc)
	if err != nil {
		logger.Error("failed to start privacy workflow worker", "error", err)
		os.Exit(1)
	}
	defer stopPrivacyWorker()

	// Phase 21: authzSvc already satisfies trustsafety.ModeratorChecker
	// directly (IsPlatformAdmin plus the new IsModerator), and
	// ratelimit.Limiter wraps the same redisClient messaging/
	// notifications already use - no new infra connection needed.
	trustSafetySvc := trustsafety.NewService(trustsafetypg.New(pool), authzSvc, ratelimit.New(redisClient))

	// Phase 22: stripe.Provider is the one PaymentProvider this phase
	// ships as genuinely live-testable (real HMAC-SHA256 webhook
	// signature verification, no live Stripe account needed) - see
	// internal/billing/README.md for why Apple IAP/Google Play are
	// deliberately not registered here.
	billingSvc := billing.NewService(billingpg.New(pool), authzSvc)
	billingSvc.RegisterProvider(stripe.New(stripe.Config{WebhookSecret: cfg.StripeWebhookSecret}))

	// Phase 23: Track is this platform's one deliberately open,
	// unauthenticated write endpoint (see internal/api/router.go's
	// comment on RegisterTrackRoute) - rate-limited by IP via the same
	// ratelimit.Limiter type Phase 21 introduced, reused for an
	// unrelated reason here (no Bearer token to key a limiter by).
	// core-api never queries analytics_events for analytics purposes -
	// backend/worker's analyticspipeline is what actually ships batches
	// downstream; see internal/analytics/README.md.
	analyticsSvc := analytics.NewService(analyticspg.New(pool), authzSvc, ratelimit.New(redisClient))

	handler := otelx.Wrap(serviceName, api.New(cfg, apps, identitySvc, usersSvc, devicesSvc, authzSvc, tenantsSvc, relationshipsSvc, groupsSvc, messagingSvc, notificationsSvc, filesSvc, searchSvc, jobsSvc, workflowsSvc, featuresSvc, remoteConfigSvc, auditSvc, privacySvc, trustSafetySvc, billingSvc, analyticsSvc))
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
