package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/core-api/internal/api"
	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationspg "github.com/example/core-platform/backend/core-api/internal/applications/postgres"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/authz/openfga"
	authzpg "github.com/example/core-platform/backend/core-api/internal/authz/postgres"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	devicespg "github.com/example/core-platform/backend/core-api/internal/devices/postgres"
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
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	relationshipspg "github.com/example/core-platform/backend/core-api/internal/relationships/postgres"
	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	tenantspg "github.com/example/core-platform/backend/core-api/internal/tenants/postgres"
	"github.com/example/core-platform/backend/core-api/internal/users"
	userspg "github.com/example/core-platform/backend/core-api/internal/users/postgres"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
	workflowspg "github.com/example/core-platform/backend/core-api/internal/workflows/postgres"
	workflowstemporal "github.com/example/core-platform/backend/core-api/internal/workflows/temporal"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
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
	authzSvc := authz.NewService(authzpg.New(pool), openfgaProvider)
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

	handler := otelx.Wrap(serviceName, api.New(cfg, apps, identitySvc, usersSvc, devicesSvc, authzSvc, tenantsSvc, relationshipsSvc, groupsSvc, messagingSvc, notificationsSvc, filesSvc, searchSvc, jobsSvc, workflowsSvc))
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
