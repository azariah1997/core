# Core Platform Go SDK

"Future products should primarily interact with Core through these SDKs" - the roadmap's own framing for this phase. `coresdk.Client` is the one entry point a Go product should ever need into core-api; `coresdk.RealtimeClient` is the equivalent for realtime-gateway.

## The roadmap's nine SDK responsibilities, and where each one lives

- **Authentication** - `TokenSource` (`auth.go`): `StaticTokenSource` for an already-minted token, `PasswordTokenSource` for a real Resource Owner Password Credentials grant against Keycloak (the same grant every phase's live validation and `apps/admin`'s server-side login already use).
- **Token refresh** - `PasswordTokenSource.Token` caches the minted token and transparently re-mints it once it's within 15 seconds of real expiry - a caller never sees an expired token or has to think about refresh at all.
- **API calls** - `Client.Do` (`coresdk.go`) is the one method every typed operation ultimately calls; `operations.go` layers typed convenience methods on top for a representative slice of the real API (platform, identity, users, devices, applications) - not all 146 registered routes. Broader coverage is real, additive work anyone can do the same way these were written (copy a `*http.go` response struct's JSON shape verbatim), not a framework limitation; `Client.Do` is always available as the untyped escape hatch for any route without a wrapper yet.
- **Errors** - `APIError` (`errors.go`) mirrors the platform's real `{code, message, correlationId}` envelope exactly (`packages/go/platformkit/apperr`'s wire format) - `IsCode(err, coresdk.CodeConflict)` is how a caller branches on a specific failure, not a status-code or string comparison.
- **Pagination** - `Page[T]`, `Paginate`, `CollectAll` (`pagination.go`) adapt to the `{items, nextCursor}` shape every list endpoint in this platform returns, so a caller writes one `visit` function instead of hand-rolling a cursor loop per endpoint.
- **Retries where safe** - literally: only `GET` is retried by default (`retry.go`), and only on transient statuses (429/502/503/504) or a network error. A `POST`/`PATCH`/`DELETE` is never retried automatically, since most routes here have no idempotency key to deduplicate a second attempt against. `WithRetries` lets a caller override this if they know better for a specific route.
- **Realtime connection** - `RealtimeClient.Dial` (`realtime.go`) reproduces realtime-gateway's actual protocol exactly: `GET /ws?access_token=...&deviceId=...` (both required), and the real `serverMessage`/`clientMessage` JSON shapes from `backend/realtime-gateway/internal/ws/messages.go` - not a guessed or simplified wire format.
- **Correlation IDs** - every `Do` call sets `X-Correlation-ID` (generated per call, or supplied via `WithCorrelationID`), matching `platformkit/correlation`'s real header name.
- **Device registration** - `Client.DevicesRegister` - called out because `RealtimeClient.Dial` needs a real registered device ID, not a free-form string, to authenticate.

## Layout

- `coresdk.go` - `Client`, `Do`, `Option`s.
- `auth.go` - `TokenSource` implementations.
- `errors.go` - `APIError`, error codes, `IsCode`.
- `retry.go` - the GET-only-by-default retry policy.
- `pagination.go` - `Page[T]`, `Paginate`, `CollectAll`.
- `realtime.go` - `RealtimeClient`, `RealtimeConn`.
- `operations.go` - typed convenience methods (platform/identity/users/devices/applications).
- `*_test.go` - unit tests against `httptest.Server`, covering auth header attachment, real error-envelope decoding, retry behavior (including the GET-only default, proven by asserting a `POST` against a failing server gets exactly one attempt), token refresh-near-expiry, and pagination.

## Live validation

A throwaway `cmd/livecheck/main.go` (not committed, the same pattern every phase's manual verification in this repo has used) exercised the whole SDK against real running Keycloak/core-api/realtime-gateway: minted a real password-grant token, called `GetPlatform`/`IdentityMe`/`UsersMe`/`UsersUpdateMe`, registered and listed and revoked a real device, paginated every real application via the generic `Paginate` helper, confirmed a nonexistent user decodes into a real `RESOURCE_NOT_FOUND` `*APIError`, and opened two real WebSocket connections that subscribed to the same channel and confirmed a genuine cross-connection fan-out (not mocked) - `conn2` publishing was correctly rejected until it also subscribed, the real server-side rule (`internal/ws/handler.go`: "must subscribe before publishing to a channel"), which the live check now exercises correctly. Found and fixed one real bug this way: `Platform`'s fields were named `Env`/`Version`, but the server actually returns `environment`/`apiVersion` - corrected after checking the real response body, not assumed from the field names that "sounded right."
