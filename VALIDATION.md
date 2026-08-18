# Validation Report

Generated validation for this boilerplate in the creation environment.

## Passed
- Repository/config required-artifact checks
- Secret-pattern sanity check
- OpenAPI/AsyncAPI/Protobuf contract presence checks
- SQL migration/outbox static check
- Argo CD and Helm scaffold static check
- YAML parsing for Compose, observability, Argo, Backstage catalog and API contracts
- `gofmt`
- `go test` across all Go workspace modules
- `go build` across all Go workspace modules
- Live Core API health and platform endpoints
- Live Realtime Gateway health endpoint
- Live RFC6455 WebSocket upgrade handshake
- Live Worker health endpoint

## Not executable in the creation environment
The environment used to generate this repo does not contain Docker, Flutter, Terraform, kubectl or Helm and has no AWS/Cloudflare/App Store/Google Play credentials. Therefore these commands are provided but were not falsely marked as executed:

- `make local-up`
- `make flutter-get && make flutter-test`
- `make terraform-validate`
- `make helm-lint`
- real AWS/Kubernetes/Cloudflare deployment
- iOS/Android signing and store deployment

Run `make doctor` on the target Mac after `scripts/bootstrap-macos.sh` to confirm prerequisites.

## Important scope
This is a production-oriented **boilerplate/foundation**, not a completed hosted platform. Infrastructure components, contracts, module boundaries and deploy paths are wired and documented. Domain implementations such as full Keycloak login flows, OpenFGA policies, PostgreSQL repositories, Kafka producers/consumers, billing providers and notification providers are intentionally extension points for the platform modules rather than fake implementations.

## 2026-08-17: Phase 1 Foundation Audit

Ran on a real Mac dev environment (Docker, Flutter, Terraform, kubectl, Helm all installed), so the commands the creation environment could only provide, not execute, were actually run here:

### Passed
- `make doctor`, `gofmt -l` (clean), `go vet` (per module), `go build`, `go test` — all services and `platformkit`
- `python3 scripts/validate_config.py`, `python3 scripts/validate_contracts.py`
- `terraform -chdir=infra/terraform validate`
- `terraform -chdir=infra/terraform fmt -recursive -check`
- `helm lint infra/kubernetes/charts/core-platform`
- `make local-up` — all 13 containers healthy
- `make flutter-get`; Next.js admin `npm install && npm run build`
- Live smoke tests (`make smoke`) against real `core-api`/`realtime-gateway`/`worker` processes
- Manual raw WebSocket handshake against `realtime-gateway` after wrapping its handler in `otelhttp` middleware (confirms `http.Hijacker` still passes through)
- End-to-end OpenTelemetry trace export verified against the real collector/Tempo containers (not just build-time compilation)

### Fixed during this audit
- `infra/terraform/versions.tf`: alignment fix so `terraform fmt -check` (run in CI) passes
- `infra/docker/docker-compose.yml`: `temporal` depended on `postgres` starting, not being ready — added `condition: service_healthy` to prevent a schema-setup crash race
- `infra/observability/tempo.yaml` and `infra/observability/otel-collector.yaml`: OTLP receivers defaulted to binding `localhost` inside their containers, making them unreachable from other containers on the Docker network (and unreliable over IPv6 loopback even from the host) — explicitly bound both to `0.0.0.0`

## 2026-08-17: Phase 2 - Application Registry

First real domain module: `Application` CRUD (`POST/GET/PATCH /v1/apps`, `GET /v1/apps/{id}`) with cursor pagination, slug validation/uniqueness, and `application.created`/`application.updated` events written transactionally to `outbox_events` alongside the row change (verified live: both the row and its outbox event land in the same Postgres transaction).

Validated live against the real `docker-compose` Postgres, not just the in-memory fake used by unit tests: create, get, list with two-page cursor pagination, patch, duplicate-slug 409, invalid-slug 400, unknown-id 404, and the outbox rows themselves (`SELECT * FROM outbox_events`) all confirmed by hand. `data/migrations/0002_applications_registry.sql` adds the `updated_at` column the domain model needs.

Environment/ApplicationVersion/ApplicationConfiguration (also named in the Phase 2 roadmap) are intentionally deferred - documented as such in `backend/core-api/internal/applications/README.md` - since no consumer needs them yet and the roadmap itself only specifies concrete endpoints for `Application`.

## 2026-08-17: Phase 3 - Identity

`Provider` interface (`ValidateToken`/`CreateIdentity`/`DisableIdentity`/`GetIdentity`) with a real Keycloak-backed implementation - JWKS-verified JWT validation, plus all three admin operations against Keycloak's real Admin REST API (not stubs). Platform-side `identities` linkage table records `(provider, providerSubject)` -> status/`last_login_at`, atomically upserted on every authenticated request so "validate" and "remember this login" can't happen separately.

Since the Keycloak container had no working realm before this (`JWT_ISSUER` pointed at `realms/core`, which didn't exist - only the default `master` realm did), added `infra/keycloak/realm-core.json`, imported via `--import-realm` on every `make local-up` (Keycloak's dev storage has no persistent volume, so this isn't a one-time migration - it's expected to run every restart). Hit and fixed a real Keycloak 26 gotcha along the way: `VERIFY_PROFILE` is enabled at the realm level by default and blocks the password grant with an opaque "Account is not fully set up" error even when the user's own `requiredActions` list is empty; had to explicitly disable it in the realm import.

Validated fully live, not just against the in-memory fake: minted a real token from Keycloak's token endpoint, confirmed `GET /v1/identity/me` returns 401 with no token / a garbage token and 200 with the real one, confirmed the `identities` row lands in Postgres with the correct `provider_subject`, confirmed a second call reuses the same row rather than duplicating it, and separately exercised `CreateIdentity`/`GetIdentity`/`DisableIdentity` end-to-end against the real Keycloak Admin API (create a user, fetch it, disable it, confirm `enabled: false` on re-fetch) via a throwaway verification program (not committed - Admin-API-backed methods aren't wired to an HTTP route yet, so there's nothing in the default test suite that would exercise them against a live Keycloak).

`users.identity_subject` (from the original `0001_core.sql` scaffold) is superseded by the new `identities` table but intentionally left alone - dropping/renaming it is a non-additive schema change with no current reader depending on the new table yet, per this repo's "prefer additive changes" migration rule.

## 2026-08-17: Phase 4 - Users

The platform person/account, separate from Identity: `GET/PATCH /v1/users/me`, `GET /v1/users/{id}`. Ties Phases 3 and 4 together via `Service.EnsureForIdentity` - resolves the User already linked to an authenticated Identity, or provisions and links one on first login, with the display name derived from the token's `preferred_username`/`email` claims. Full soft-delete lifecycle: `active` <-> `deactivated` via the normal `PATCH`, `deleted` only reachable through a dedicated `Service.Delete` (rejected with 400 if attempted via `PATCH .../status`) so it's never accidental and always emits its own `user.deleted` event rather than a generic `user.updated`.

`users` and `identity` stay decoupled in both directions: `users` never imports `identity` (`EnsureForIdentity` takes primitive values and a small `IdentityLinker` interface it defines itself, satisfied structurally); the composition - reading identity's context, deriving a display name, calling `EnsureForIdentity` - lives in `internal/api/session.go`, not in either domain package.

`0004_users_registry.sql` adds `avatar_ref` and relaxes `identity_subject` to nullable (superseded by `identities.user_id`, which supports multiple linked identities per user where the old column only supported one - left in place rather than dropped, per this repo's additive-migration preference).

Validated live end-to-end against real Postgres + Keycloak: no token -> 401 on both `/me` endpoints, first authenticated request provisions and links a User (confirmed via `SELECT` on `identities.user_id`), a second request resolves the same User, `PATCH` updates fields and toggles `active`/`deactivated`, attempting `status: deleted` via `PATCH` is correctly rejected, and all four event types (`user.created`, `user.updated`, `user.deactivated`, `user.deleted`) confirmed landing in `outbox_events`. `Service.Delete` isn't behind an HTTP route yet (no endpoint for it in this phase's scope), so it was verified separately via a throwaway (not committed) program against real Postgres, the same way Phase 3's Keycloak Admin API methods were.

## 2026-08-17: Phase 5 - Devices/Sessions

Per-user device tracking: `POST/GET /v1/users/me/devices`, `DELETE /v1/users/me/devices/{id}`, all scoped to the caller's own user (enforced at the repository query level, not just routing). `Register` upserts by `(userID, clientDeviceID)` - first sight creates, repeat sight refreshes profile fields and `LastActiveAt`, and re-registering a previously revoked device reactivates it. `DELETE` soft-revokes (`session_status = 'revoked'`) rather than hard-deleting, so a device disappears from the active list while the row is retained.

No separate `Session` table: `Device.SessionStatus` (`active`/`revoked`) stands in for it, since nothing in this phase needs more than one live session per device - documented as the deferred extension in `backend/core-api/internal/devices/README.md`, same pattern as this repo's other intentionally-scoped-down phases. No events emitted either, matching the roadmap (Phase 5, unlike Phases 2/4, specifies none).

`0005_devices_sessions.sql` adds `client_device_id`, `os_version`, `locale`, `timezone`, `session_status` and the `(user_id, client_device_id)` uniqueness the upsert depends on - safe as a plain (non-partial) unique index since the `devices` table had zero rows before this phase.

Validated live end-to-end against real Postgres + Keycloak: 401 with no token, registered two devices, listed both, revoked one (204) and confirmed it disappeared from the list while a second revoke attempt correctly 404s, re-registered the revoked device and confirmed it reactivated under the *same* device ID with its previous push token preserved (via `COALESCE` in the upsert - re-registering without repeating `pushToken` doesn't wipe it), and confirmed the response body never contains the raw push token string at any point.

## 2026-08-17: Phase 6 - Authorization / Access

The last of the six foundation phases. `authz.Service.Can(subject, action, resource)` - the literal signature the roadmap asked for - is now the one entry point every domain module uses instead of implementing permission logic independently. Two layers, as specified: RBAC roles (`platform.admin`, `support`, `moderator`) in Postgres, and a real OpenFGA-backed fine-grained `Provider` for `Can`/`Grant`/`Revoke`. `AssignRole(platform.admin)` writes to both - the role in Postgres and the equivalent `admin` relation on a singleton `platform:core` object in OpenFGA - so `Can`/`IsPlatformAdmin` see it without a caller needing to know both systems exist.

First concrete consumer: `GET /v1/users/{id}`, previously authenticated-only (documented as a known gap in the Phase 4 entry above), now enforces "self or `platform.admin`" through `authz`, via a `users.AccessChecker` interface `users` defines but doesn't implement (composed in `internal/api/authz_adapter.go`, keeping `users` decoupled from `authz` the same way it's decoupled from `identity`).

Two real bugs found and fixed via live testing against the actual OpenFGA server (not assumed from the SDK docs):
- The `CanViewProfile` policy originally had a speculative fallback `Can()` call against an undefined `user_profile`/`view` relation, intended as "where Phase 8 plugs in" - but `Check()`-ing a type/relation that doesn't exist in the model is a genuine OpenFGA error, not a graceful `false`, and broke every cross-user request with a 500. Removed; Phase 8 will add the model and the check together once there's real relationship data.
- `AssignRole`/`RevokeRole` are documented (and relied on) as idempotent, but the OpenFGA server version this repo runs locally (`openfga/openfga:v1.9.5`) doesn't honor the Go SDK's built-in `Conflict.OnDuplicateWrites`/`OnMissingDeletes` write options despite the SDK sending them - confirmed directly via `curl` against the write API, not assumed. `openfga.Provider.Grant`/`Revoke` now detect the resulting `write_failed_due_to_invalid_input` error via the SDK's typed `FgaApiValidationError` and treat it as success, since in context (Grant only ever writes, Revoke only ever deletes) it always means "this was already a no-op."

OpenFGA's `memory` datastore is exactly as ephemeral as Keycloak's dev-mode storage (confirmed: `{"stores":[]}` after every container restart) - `openfga.Provider`'s constructor finds-or-creates its store and authorization model by name at startup, self-healing the same way the Keycloak realm import does.

Validated live end-to-end against real Postgres + Keycloak + OpenFGA: two distinct Keycloak users, self-view via `/v1/users/{id}` (200), cross-user view without a role (403, `ACCESS_DENIED` envelope), granted `platform.admin` and confirmed cross-user view now succeeds (200) - cross-checked at every layer: the HTTP response, the `user_roles` Postgres row, and the actual OpenFGA tuple via `POST /stores/{id}/read`. Also separately verified the idempotent grant/revoke/revoke-again cycle from both starting states (already-granted and not-yet-granted) via a throwaway (not committed) program, the same pattern used for Phase 3/4's admin-API/delete verification.

## 2026-08-17: Phase 7 - Tenants / Organizations

The reusable multi-tenancy boundary: `Tenant` + `Membership` (`owner`/`admin`/`member`), scoped to an Application (Phase 2). This phase had the least concrete guidance of any so far - no example endpoints anywhere in the roadmap, unlike every prior phase - so both the endpoint set (`POST/GET /v1/tenants`, `GET/PATCH /v1/tenants/{id}`, `GET/POST /v1/tenants/{id}/members`, `DELETE .../members/{userId}`) and the decision to treat "Tenant" and "Organization" as one resource (rather than two structurally-redundant tables, given the roadmap gives Organization no distinct fields) are this pass's own scoping, documented as such in `backend/core-api/internal/tenants/README.md`.

Deliberately does *not* route tenant-role checks ("is this user an owner/admin of this tenant") through `authz.Service`/OpenFGA, even though Phase 6 just built that - answered directly from `Membership` data instead, since it's this package's own data, not the kind of cross-domain relationship question `authz` exists for. Using the generic ReBAC layer for a plain membership lookup would have been reaching for infrastructure the requirement didn't call for.

Validated live end-to-end against real Postgres + Keycloak with two distinct users and a real Application: create (owner auto-membership), non-member GET (403), non-member trying to add themselves as a member (403, can't self-escalate), owner adding the second user as a plain member (201), that member's GET now succeeding (200), that member's PATCH attempt on the tenant correctly denied (403, `owner or admin role required`), `GET /v1/tenants` correctly scoped to only the caller's own tenants, and self-removal ("leave") succeeding and immediately revoking access - cross-checked against the actual `tenant_memberships` row count in Postgres, not just the HTTP response.

## 2026-08-17: Phase 8 - Relationships / Social Graph

A generic relationship graph: `request`/`accept`/`decline`/`remove`/`block`, exactly the five actions the roadmap named, built on the `relationships` table that already existed in the original scaffold (`app_id`, `user_a`/`user_b`, `relationship_type`, `status`, `metadata`) - this package treats `user_a`/`user_b` as requester/target by its own convention rather than renaming the columns, since the table's `LEAST`/`GREATEST` uniqueness constraint doesn't care which side holds which. `RelationshipType` stays product-defined throughout (a plain string never validated against a fixed platform enum), matching the roadmap's explicit "do not encode partner/friend/follower as fixed platform behaviour."

Three genuine bugs found and fixed via live testing:
- `writeDomainError`'s `default` case in `devices`, `tenants`, and `relationships` (everything built since Phase 2's `applications`) was silently swallowing unexpected errors as a bare 500 with no server-side log line - `applications` had the fix, the other three didn't. All three now log via `slog` with the correlation ID, matching `applications`.
- `jsonb` `metadata` scanning into `map[string]any` was unverified until tested live - confirmed working via pgx v5's built-in codec, no manual `json.Unmarshal` needed.
- Originally, `Request` returned a conflict for *any* existing row regardless of status, including `ended` - meaning once two users' relationship ended, that pair could never request again, for that type, ever. Real products need "unfriend, then re-friend later" to work; fixed by adding `Repository.Revive`, which resets an ended row back to `pending` under the new requester/target instead of insisting on a fresh row. Verified live: the exact same relationship ID flipped from `ended` back to `pending` with the requester/target correctly swapped.

Validated live end-to-end against real Postgres + Keycloak with three distinct users and a real Application: request with `jsonb` metadata, requester attempting to accept their own request (403), target accepting (200, active), accepting again (409, already active), get/list scoped to participants, remove/unfriend by either participant (204, confirmed `ended` in Postgres), blocking a stranger with no prior relationship (creates a row directly in `blocked`), the revive path (re-requesting after `ended` reuses the same row), and `relationship.created` landing in `outbox_events` for both a normal request and a direct-to-`blocked` row.

## 2026-08-17: Phase 9 - Groups / Circles

A generic grouping primitive: `Group` + `GroupMember`, deliberately not hardcoded to one use case (friend circles, families, teams, communities, workspaces, gaming guilds are all the same shape). Unlike Phase 7's `Tenant` (a fixed owner/admin/member hierarchy, justified there since tenancy is an infrastructure/access-control concept), `GroupMember.Role` here is a free-form, product-defined label never validated against an enum or used for authorization - the platform enforces exactly one structural bit, `IsManager`, orthogonal to whatever the product calls that member's role. Any manager can promote/demote any other member's `IsManager` bit, including their own or the creator's - no "owner outranks admin" hierarchy is invented, since the roadmap never asked for one; a product that wants that layers it on top of this primitive.

Validated live end-to-end against real Postgres + Keycloak with two users and a real Application: created a group with the creator auto-granted manager status, non-member GET denied (403), added a second user with the free-form role label `"child"` (confirmed stored and returned verbatim, not coerced into any fixed set), that plain member's attempt to update the group and to self-promote both correctly denied (403), the creator promoting them to manager with a different free-form label (`"parent"`), confirmed the newly-promoted manager could then update the group themselves, and self-removal ("leave") succeeding and confirmed against the actual `group_members` row count in Postgres.

### Known gap, not fixed (out of scope for this pass)
- `UserPreferences`, named in the Phase 4 roadmap heading, has no concrete fields specified anywhere in the roadmap and no consumer yet - deferred to Phase 12 (Notifications), which is the first phase that actually needs opt-outs/quiet-hours data, rather than inventing a shape now.
- `GET /v1/users/{id}` is authenticated-only today (any logged-in caller can view any user's profile) - resource-scoped visibility is explicitly Phase 6's job.
- `apps/mobile` has no `test/` directory, so `make flutter-test` fails; not currently wired into CI either. Deferred to when the mobile shell gets real screens worth testing.
- `apps/admin`'s `next@15`/`sharp` pulls in 3 high-severity transitive advisories (postcss, libvips); the fix is a Next.js major-version bump, which is a separate, reviewable change.
- Real PostgreSQL/Redis/Kafka client wiring (as opposed to the TCP-reachability health checks that exist today) is deferred to whichever domain module first needs it, per this file's existing scope note above.
