# Identity

"Who authenticated" - deliberately separate from User ("the platform person/account", Phase 4). Provider-neutral: platform code depends on `Provider` and `Repository`, never on Keycloak directly, so Google/Apple/Microsoft/passkeys are additional `Provider` implementations later, not a rewrite.

## Responsibilities

- `Provider`: validate a bearer token (real JWT/JWKS verification against the configured IdP) and manage identities at the provider (create/disable/get). Not every provider can meaningfully implement every method - a federated provider (Google, Apple) has no `CreateIdentity` of its own and returns `ErrUnsupportedOperation`.
- `Repository`: the platform's own linkage record - `(provider, providerSubject)` -> platform `Identity`, with status and `last_login_at`. `Touch` is the one place "validate a token" and "remember this login" happen atomically (an upsert, not a read-then-write).
- `Middleware`: requires a valid bearer token, resolves it to an `Identity`, attaches it to the request context.

## Non-responsibilities

- No login/signup UI or OAuth redirect flow - this validates tokens a client already obtained from the IdP directly (e.g. Keycloak's own token endpoint), it doesn't broker the browser flow.
- Does not create or link a platform `User` - `identities.user_id` is nullable; an identity can authenticate before it's provisioned as a User. `LinkUser` records the linkage once someone else decides it; `internal/users` (Phase 4) does the deciding, via `internal/api/session.go`'s composition.
- Does not decide which endpoints require authentication - `Middleware` is applied per-route by whoever registers routes, not globally.

## Layout

- `domain.go` - `Identity`, `Claims`, `Provider` and `Repository` interfaces.
- `service.go` - `Service.Authenticate` (validate + touch, returning both the persisted `Identity` and the raw `Claims` - the latter needed only by one-time decisions like naming a newly provisioned User) and `LinkUser`. The only thing HTTP/SDK layers should depend on.
- `http.go` - `Middleware` and `GET /v1/identity/me` (a minimal endpoint proving the whole pipeline, independent of any product feature needing auth yet).
- `keycloak/` - the production `Provider`: JWKS-verified JWT validation plus Keycloak Admin REST API-backed `CreateIdentity`/`GetIdentity`/`DisableIdentity`.
- `postgres/` - the production `Repository`, atomic upsert via `ON CONFLICT`.
- `memory/` - in-memory `Repository` and a fake `Provider` (token string == subject, no verification) for tests.

## Storage

`identities` table (`data/migrations/0003_identities.sql`): `(provider, provider_subject)` unique, `user_id` nullable, `status`, `created_at`, `last_login_at`.

## Local dev

`infra/keycloak/realm-core.json` is imported into Keycloak on every `make local-up` (Keycloak's dev-mode storage is ephemeral, so this runs every restart, not just the first). It defines the `core` realm, a public `core-platform` client with direct-access-grants enabled, and a `demo`/`demo` user - enough to mint a real token locally without a browser:

```bash
curl -X POST http://localhost:8081/realms/core/protocol/openid-connect/token \
  -d client_id=core-platform -d grant_type=password -d username=demo -d password=demo
```
