# Access / Authorization

The platform's single authorization service. Every other domain module depends on this instead of implementing permission logic independently, per the platform's non-negotiable rule.

## Responsibilities

- **RBAC** (`Role`, `RoleRepository`): coarse, platform-wide roles (`platform.admin`, `support`, `moderator`). Role assignment is the platform's own data - never delegated to `Provider`, since an external engine shouldn't own who counts as an admin.
- **Fine-grained permissions** (`Provider`, `Can`): "may subject `<action>` `<resource>`?", backed by OpenFGA. `Can` is the literal `Can(subject, action, resource)` entry point every domain module should call instead of writing its own permission logic.
- `AssignRole`/`RevokeRole` keep both layers in sync for roles that imply a fine-grained grant: assigning `platform.admin` also grants the `admin` relation on the singleton `platform:core` object in OpenFGA, so `Can`/`IsPlatformAdmin` see it without a caller needing to know both systems exist.

## Non-responsibilities

- No per-resource-type OpenFGA model beyond the platform-wide `admin` relation yet. Adding one (e.g. a `user_profile` type with a `viewer` relation) is Phase 8's job (Relationships), done together with the relationship data it would check against - `Check()`-ing an undefined type/relation is a genuine OpenFGA error, not a graceful "false", so this package doesn't speculate ahead of real data.
- Doesn't decide product-specific policies (e.g. "friends can see friends-only content") - it provides the primitives (`HasRole`, `Can`); composing them into a policy for a specific endpoint is that endpoint's caller's job (see `internal/api/authz_adapter.go` for `users`' "self or platform.admin" policy).

## Layout

- `domain.go` - `Role`, `Action`, `Resource`, `Provider` and `RoleRepository` interfaces.
- `service.go` - `Service`: `HasRole`, `AssignRole`, `RevokeRole`, `Can`, `IsPlatformAdmin`.
- `openfga/` - the production `Provider`. Finds or creates its OpenFGA store and authorization model by name at startup (self-healing across restarts of OpenFGA's ephemeral in-memory datastore, the same problem Keycloak's realm import solves). `Grant`/`Revoke` are idempotent by design (a caller can call them more than once for the same grant) - the OpenFGA server version this repo runs locally doesn't honor the SDK's built-in `on_duplicate`/`on_missing` write options despite sending them, so idempotency is handled by inspecting the resulting error instead (confirmed live, not assumed).
- `postgres/` - the production `RoleRepository`.
- `memory/` - in-memory `RoleRepository` and `Provider` for tests.

## Storage

`user_roles` table (`data/migrations/0006_authorization.sql`) plus whatever OpenFGA's own store holds - this repo never reaches into OpenFGA's storage directly, only through its API.
