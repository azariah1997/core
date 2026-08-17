# Application Registry

The platform's generic record of which applications exist. This is Core Platform's own bookkeeping (Phase 2 of the roadmap) - it is not a per-product feature and must never gain product-specific fields.

## Responsibilities

- Own the `Application` resource: `id`, `slug`, `name`, `status`, `createdAt`, `updatedAt`.
- Enforce slug format/uniqueness and status transitions server-side.
- Emit `application.created` / `application.updated` via the transactional outbox, atomically with the write.

## Non-responsibilities

- Environments, application versions and per-application configuration are named in the platform roadmap but not implemented here yet; they are separate concerns to add when a real consumer needs them, not implied by this package.
- Does not publish to Kafka directly - a worker polling `outbox_events` owns that (not yet built).
- Does not perform authorization; callers (HTTP handlers today, an SDK/gRPC surface later) are responsible for who is allowed to call `Service`.

## Layout

- `domain.go` - `Application`, `Repository` interface, validation.
- `service.go` - the only thing outside callers should depend on.
- `http.go` - REST handlers (`POST/GET/PATCH /v1/apps...`), mapped onto `apperr`'s standard error shape.
- `postgres/` - the production `Repository`, using the transactional outbox pattern.
- `memory/` - an in-memory `Repository` for tests (and anything else that wants the registry without a database).

## Storage

`applications` table (`data/migrations/0001_core.sql`, `0002_applications_registry.sql`) plus a row per event in the shared `outbox_events` table. List pagination is cursor-based (`created_at, id`), never offset-based.

## Contracts

`contracts/openapi/core-api.yaml` (`/v1/apps*`), `contracts/asyncapi/events.yaml` (`application.created.v1`, `application.updated.v1`).
