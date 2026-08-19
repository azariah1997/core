# Admin Portal

A Next.js 15 App Router UI that turns the roadmap's 18 named "visibility areas" (plus billing and AI Gateway, real but not roadmap-named) into an actual browsable console backed entirely by live core-api calls - no mock data, no direct database access (this app has no database credentials configured at all, so there is nothing to bypass service APIs with even if a page tried).

## Auth: a real Keycloak session, deliberately scoped down

`lib/keycloak.ts` does a direct Resource Owner Password Credentials grant against the real `core` realm - the same auth boundary every phase's live validation has used via curl all session long, just from a Server Action instead of a shell command. The session cookie (`lib/session.ts`, `core_admin_session`, httpOnly/sameSite=lax) stores the raw signed Keycloak access token directly rather than wrapping it in a second signature layer, since the JWT's own signature already defeats a tampered-cookie threat. `middleware.ts` redirects any request without that cookie to `/login` at the edge, before a single Server Component runs.

A full OIDC Authorization Code + PKCE flow is the right choice for a public-facing app, but needs a redirect URI, a callback route, and silent-refresh machinery this internal, small-trusted-user-base tool doesn't need - the same "don't build for a hypothetical future requirement" bar every backend phase in this repo has held itself to.

## Every page's data is real, or says exactly why it isn't

`components/nav.ts` marks each area `live: true|false`. `live: false` renders `components/ComingSoon.tsx` - a named reason, never fake rows. Every deferred area shares the same real cause: the backing module only exposes a `listMine`-style, self-scoped endpoint (`devices`, `tenants`, `relationships`, `messaging`, `notifications`, `files` all work this way), not an admin-wide listing - the same gap `GET /v1/users` closed for `users` this phase, but closing it for six more modules at once was out of scope here. `Permissions` and `Deployments` are documentation-style and roadmap-scheduling pages respectively, not data tables.

Two small backend additions this phase made real, not just visible:
- `GET /v1/users` (`backend/core-api/internal/users/http.go`) - the platform's first admin-wide user listing, cursor-paginated, gated by a new `IsPlatformAdmin` check on `AccessChecker`.
- `authz`'s first-ever HTTP surface (`backend/core-api/internal/authz/http.go`) - `GET/POST /v1/authz/roles`, `POST /v1/authz/roles/revoke` - replacing the throwaway `cmd/regrant-*/main.go` Go programs every earlier phase's live validation needed to grant itself a role. `Service.RolesFor` allows self-lookup or platform.admin; `AssignRole`/`RevokeRole` already required an acting-user ID from Phase 19's audit work.

## Layout

- `middleware.ts` - cookie-presence redirect gate.
- `app/login/` - `page.tsx` (Client Component form, `useActionState`), `actions.ts` (the real Keycloak grant + `setSession`).
- `app/logout/route.ts` - clears the session cookie.
- `app/(portal)/layout.tsx` - the shared session check + sidebar + topbar; a route group so it applies to every real page without a `/portal` URL prefix.
- `app/(portal)/*/page.tsx` - one per nav item; see `components/nav.ts` for the full live/deferred map.
- `app/(portal)/users/[id]/` - the one page with a nested Server Action file (`actions.ts`) and Client Component (`RoleManager.tsx`), since role grant/revoke needs real interactivity, not just a read.
- `lib/api.ts` - the single place this app talks to core-api; every typed function attaches the real session's Bearer token via `apiFetch`.
- `lib/session.ts`, `lib/keycloak.ts` - the auth boundary described above.

## Environment

Every value defaults to this repo's real local dev addresses (`http://localhost:8080` for core-api, `8090` realtime-gateway, `8091` worker, `8081` Keycloak, realm `core`, client `core-platform`) - matching root `.env` exactly, so no `.env.local` is required for local development. Override via `CORE_API_URL`, `REALTIME_HEALTH_URL`, `WORKER_HEALTH_URL`, `KEYCLOAK_URL`, `KEYCLOAK_REALM`, `KEYCLOAK_CLIENT_ID` for any other environment.

## Not done here

Sessions & Devices, Tenants, Relationships, Messages, Notifications, and Files are real backend modules with no admin-wide listing endpoint yet - each `ComingSoon` page names that specific gap. Deployments has nothing to show until Phase 29 (CI/CD, Helm releases, rollout status) gives it real data. Permissions documents the RBAC (`authz` roles, managed here) + ReBAC (relationship-tuple engine, not yet browsable) model rather than rendering a table.
