# Users

The platform person/account - deliberately separate from Identity ("who authenticated", Phase 3). No product-specific data belongs here, only the generic profile fields every product needs.

## Responsibilities

- Own the `User` resource: `id`, `displayName`, `avatarRef`, `locale`, `timezone`, `status`, `createdAt`, `updatedAt`.
- Soft-delete/account lifecycle: `active` <-> `deactivated` freely; `deleted` is one-way and only reachable via `Service.Delete`, never via generic `Update` (so it's never accidental and always emits its own `user.deleted` event instead of a generic `user.updated`).
- `EnsureForIdentity`: resolve the User already linked to an authenticated Identity, or provision and link one on first login. This is the one place Phase 3 and Phase 4 meet - see `internal/api/session.go` for how it's wired to identity's `Middleware`.
- Emit `user.created` / `user.updated` / `user.deactivated` / `user.deleted` via the transactional outbox, atomically with the write.

## Non-responsibilities

- `UserPreferences` (notification opt-outs, quiet hours, etc.) is named in the platform roadmap but has no concrete fields specified anywhere yet - it's introduced when Notifications (Phase 12) actually needs it, not invented speculatively here.
- Does not decide who may view another user's profile itself - `GET /v1/users/{id}` calls an `AccessChecker` it defines but doesn't implement (see below); the actual policy (self access, `platform.admin`, and eventually relationship-based access from Phase 8) is `internal/api`'s and `authz`'s decision, per the platform's non-negotiable rule that domain modules never implement permission logic independently.
- Does not import `identity` or `authz` at all. `EnsureForIdentity` takes primitive values plus a small `IdentityLinker` interface it defines itself (satisfied structurally by `identity.Service`); `AccessChecker` is the analogous pattern toward `authz.Service`. Both are wired only in `internal/api`, so this package stays independently understandable without needing to know either exists.

## Layout

- `domain.go` - `User`, `Repository` interface, validation.
- `service.go` - `Service`, including `EnsureForIdentity`.
- `http.go` - `GET/PATCH /v1/users/me`, `GET /v1/users/{id}`. Takes `requireUser` middleware and an `AccessChecker` as parameters rather than building them, since composing identity/authz with users is `internal/api`'s job (see `session.go` and `authz_adapter.go` there), not this package's.
- `postgres/` - the production `Repository`.
- `memory/` - in-memory `Repository` for tests.

## Storage

`users` table (`data/migrations/0001_core.sql`, `0004_users_registry.sql`, which adds `avatar_ref` and relaxes the old `identity_subject` column to nullable - superseded by `identities.user_id`, not dropped).
