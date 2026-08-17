package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/api"
	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationspg "github.com/example/core-platform/backend/core-api/internal/applications/postgres"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/authz/openfga"
	authzpg "github.com/example/core-platform/backend/core-api/internal/authz/postgres"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	devicespg "github.com/example/core-platform/backend/core-api/internal/devices/postgres"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/identity/keycloak"
	identitypg "github.com/example/core-platform/backend/core-api/internal/identity/postgres"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	tenantspg "github.com/example/core-platform/backend/core-api/internal/tenants/postgres"
	"github.com/example/core-platform/backend/core-api/internal/users"
	userspg "github.com/example/core-platform/backend/core-api/internal/users/postgres"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/pg"
	"github.com/example/core-platform/packages/go/platformkit/runx"
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

	handler := otelx.Wrap(serviceName, api.New(cfg, apps, identitySvc, usersSvc, devicesSvc, authzSvc, tenantsSvc))
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
