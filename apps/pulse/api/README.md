# pulse-api

The Pulse Product API - a Go service on Core Platform infrastructure, consuming Core through `coresdk` and owning Pulse's own Postgres database. See `apps/pulse/docs/ARCHITECTURE_AUDIT.md`'s "Deployment Architecture" for why this is a separate service rather than a module inside `core-api`: Pulse is a product, not a Core capability, and its dependency direction is one-way (Pulse → Core).

## Auth

`internal/pulseauth` resolves every caller by forwarding their bearer token to Core's real `GET /v1/users/me` - Pulse never validates Keycloak JWTs itself, and gets Core's own auto-provision-on-first-login behavior for free.

## Layout

Follows Core's own layered module pattern exactly (see the root `docs/AI_CONTEXT.md`):

```
internal/pulseauth/       caller resolution middleware (calls Core, not Keycloak)
internal/pulseprofile/    domain.go / service.go / http.go / postgres / memory - Phase 1's one real module
internal/api/             router: health, metrics, CORS, correlation ID, module wiring
cmd/server/               entrypoint
migrations/               Pulse's own schema (never Core's)
```

## Running locally

```bash
# Core Platform infra + core-api must already be running (see root README/RUNBOOKS.md)

# one-time: create Pulse's own database and apply its migrations
docker exec docker-postgres-1 psql -U core -d core -c "CREATE DATABASE pulse OWNER core"
cat migrations/0001_pulse_profile.sql | docker exec -i docker-postgres-1 psql -U core -d pulse

PULSE_POSTGRES_DSN="postgres://core:core@localhost:5432/pulse?sslmode=disable" \
PULSE_HTTP_ADDR=":8096" \
CORE_API_URL="http://localhost:8080" \
go run ./cmd/server
```

`GET /healthz` on `:8096` confirms it's live. Every route under `/v1/pulse/*` requires a real Core-issued Bearer token.
