# Core Platform TypeScript SDK

"Future products should primarily interact with Core through these SDKs" - the roadmap's own framing for this phase. `CoreApi` (extending `CoreClient`) is the one entry point a TypeScript/JavaScript product should ever need into core-api; `RealtimeClient` is the equivalent for realtime-gateway. Isomorphic - built on `fetch`/`WebSocket`/`crypto.randomUUID`, all globally available in modern Node (22+) and every browser, so the same package works server-side (as `apps/admin` now does) or client-side.

## Real proof this works: `apps/admin` uses it for everything

`apps/admin/lib/api.ts` (Phase 25's hand-rolled `fetch` wrapper) was migrated to this SDK, not left as a parallel unused artifact next to it - the strongest evidence a TypeScript SDK actually works is a real product depending on it. Every exported function in that file's public surface (`listApplications`, `listUsers`, `getUser`, `listRoles`, `assignRole`, `revokeRole`, `listAudit`, `listModerationCases`, `listFeatures`, `listJobs`, `listEntitlements`, `listAiUsage`, `listConfig`) kept its exact name and return shape so no page needed to change - only what's *behind* those functions changed, from a hand-rolled `apiFetch` to `CoreApi`. Re-validated with the same real headless-browser (Playwright) test suite Phase 25 wrote: 16/17 checks pass (the one "failure" is a pre-existing assertion mismatch in the test script itself, unrelated to this migration - see `VALIDATION.md`), including a full real role grant/revoke round trip through the SDK.

## The roadmap's nine SDK responsibilities, and where each one lives

- **Authentication** - `TokenSource` (`src/auth.ts`): `StaticTokenSource` wraps an already-held token (what `apps/admin` uses - it already has the session's real Keycloak JWT from a cookie); `PasswordTokenSource` performs a real Resource Owner Password Credentials grant against Keycloak with transparent refresh, for a caller that needs to mint its own token (CLIs, tests, scripts).
- **Token refresh** - `PasswordTokenSource.token()` caches the minted token and re-mints it once within 15 seconds of real expiry, coalescing concurrent calls into a single in-flight refresh.
- **API calls** - `CoreClient.request` is the one method every typed operation ultimately calls; `CoreApi` (`src/operations.ts`) layers typed convenience methods on top for a representative slice of the real API, matching the Go SDK's scope plus everything the real Admin Portal needs. `request()` is always available as the untyped escape hatch for any route without a wrapper yet.
- **Errors** - `ApiError` (`src/errors.ts`) mirrors the platform's real `{code, message, correlationId}` envelope exactly - `ApiError.is(err, ErrorCodes.Conflict)` is how a caller branches on a specific failure.
- **Pagination** - `paginate`/`collectAll` (`src/pagination.ts`) are async generators over the `{items, nextCursor}` shape every list endpoint returns.
- **Retries where safe** - only `GET` is retried by default (`src/retry.ts`), only on transient statuses (429/502/503/504) or a network error - a `POST`/`PATCH`/`DELETE` is never retried automatically, since most routes here have no idempotency key. `DefaultRetryPolicy`'s constructor args (or a custom `RetryPolicy`) override this per client.
- **Realtime connection** - `RealtimeClient.dial` (`src/realtime.ts`) reproduces realtime-gateway's actual protocol exactly: `GET /ws?access_token=...&deviceId=...` (both required) and the real server/client message JSON shapes from `backend/realtime-gateway/internal/ws/messages.go`.
- **Correlation IDs** - every `request()` call sets `X-Correlation-ID` (generated via `crypto.randomUUID()`, or supplied via `{correlationId}`), matching `platformkit/correlation`'s real header name.
- **Device registration** - `CoreApi.devicesRegister` - called out because `RealtimeClient.dial` needs a real registered device ID to authenticate.

## Layout

- `src/client.ts` - `CoreClient`, `request`.
- `src/auth.ts` - `TokenSource` implementations.
- `src/errors.ts` - `ApiError`, `ErrorCodes`.
- `src/retry.ts` - the GET-only-by-default retry policy.
- `src/pagination.ts` - `paginate`, `collectAll`.
- `src/realtime.ts` - `RealtimeClient`, `RealtimeConn`.
- `src/operations.ts` - `CoreApi` (extends `CoreClient`) - typed convenience methods and their request/response types.
- `src/*.test.ts` - unit tests against a real local `node:http` server (Node's own test runner, `node --test`), covering the same ground as the Go SDK's suite: auth header attachment, real error-envelope decoding, GET-only retry behavior, token refresh-near-expiry, pagination.

## Building and testing

```sh
npm install
npm run build   # tsc -> dist/
npm test        # compiles src (incl. *.test.ts) to dist-test/, then node --test
```

`apps/admin` depends on this package via a local `file:` reference in its `package.json` - `dist/` must exist before `apps/admin` can resolve it, which is why `make admin-install`/`make admin-build` depend on a new `make sdk-ts-build` target rather than assuming it's already built.

## Live validation

A throwaway `livecheck.ts` (not committed, the same pattern every phase's manual verification in this repo has used) exercised the whole SDK against real running Keycloak/core-api/realtime-gateway: minted a real password-grant token, called every typed operation used above, registered/listed/revoked a real device, paginated every real application via the generic `paginate` helper, confirmed a nonexistent user decodes into a real `RESOURCE_NOT_FOUND` `ApiError`, and opened two real WebSocket connections that both subscribed to the same channel and confirmed a genuine cross-connection fan-out - correctly hitting the server's real "must subscribe before publishing" rule first (the same one the Go SDK's live validation found), fixed in the check script, not the SDK.
