# Core Platform Dart SDK

"Future products should primarily interact with Core through these SDKs" - the roadmap's own framing for this phase, named "Flutter SDK" there. Implemented as a **pure Dart package** (no Flutter SDK dependency - just `http` and `web_socket_channel`, both plain-Dart-VM-compatible) so it's usable and testable with `dart test` alone, while still being the real package `apps/mobile` (a real Flutter app) depends on via a path dependency and uses for its actual login/profile flow.

## Real proof this works: `apps/mobile` uses it

`apps/mobile/lib/main.dart`'s `LoginPage`/`ProfilePage` mint a real Keycloak token via `PasswordTokenSource` and fetch the caller's real profile + register a device via `CoreApi` - the same "core identity" flow the Go and TypeScript SDKs' live checks exercise. `apps/mobile/test/login_page_test.dart` proves the widget behavior (form rendering, successful-login navigation, a real auth failure showing the actual error message) using `package:http/testing.dart`'s `MockClient` - the framework's own documented answer to a real, live-discovered constraint: Flutter's widget-test binding (`TestWidgetsFlutterBinding`) forces every real `HttpClient` request to fail with a 400 by design, specifically to catch accidental real network calls in a widget test. This closes a real, previously-documented gap - `apps/mobile`'s `test/` directory didn't exist before this phase, so `make flutter-test` failed outright (see this repo's Phase 9 todo history); it now runs 3 real passing tests.

## The roadmap's nine SDK responsibilities, and where each one lives

- **Authentication** - `TokenSource` (`lib/src/auth.dart`): `StaticTokenSource` for an already-held token; `PasswordTokenSource` for a real Resource Owner Password Credentials grant against Keycloak, the same grant every phase's live validation in this repo has used.
- **Token refresh** - `PasswordTokenSource.token()` caches the minted token and re-mints it once within 15 seconds of real expiry, coalescing concurrent calls into a single in-flight refresh.
- **API calls** - `CoreClient.request` is the one method every typed operation ultimately calls; `CoreApi` (`lib/src/operations.dart`) layers typed convenience methods on top for the same representative "core identity" slice (platform, identity, users, devices, applications) the Go and TypeScript SDKs cover. `request()` is always available as the untyped escape hatch for any route without a wrapper yet.
- **Errors** - `ApiError` (`lib/src/errors.dart`) mirrors the platform's real `{code, message, correlationId}` envelope exactly - `ApiError.isCode(err, ErrorCodes.conflict)` is how a caller branches on a specific failure.
- **Pagination** - `paginate`/`collectAll` (`lib/src/pagination.dart`) stream the `{items, nextCursor}` shape every list endpoint returns.
- **Retries where safe** - only `GET` is retried by default (`lib/src/retry.dart`), only on transient statuses (429/502/503/504) or a network error - a `POST`/`PATCH`/`DELETE` is never retried automatically, since most routes here have no idempotency key.
- **Realtime connection** - `RealtimeClient.dial` (`lib/src/realtime.dart`) reproduces realtime-gateway's actual protocol exactly: `GET /ws?access_token=...&deviceId=...` (both required) and the real server/client message JSON shapes.
- **Correlation IDs** - every `request()` call sets `X-Correlation-ID` (a random 32-hex-char ID, or supplied explicitly), matching `platformkit/correlation`'s real header name.
- **Device registration** - `CoreApi.devicesRegister` - called out because `RealtimeClient.dial` needs a real registered device ID to authenticate.

## Layout

- `lib/src/client.dart` - `CoreClient`, `request`.
- `lib/src/auth.dart` - `TokenSource` implementations.
- `lib/src/errors.dart` - `ApiError`, `ErrorCodes`.
- `lib/src/retry.dart` - the GET-only-by-default retry policy.
- `lib/src/pagination.dart` - `Page<T>`, `paginate`, `collectAll`.
- `lib/src/realtime.dart` - `RealtimeClient`, `RealtimeConn`.
- `lib/src/operations.dart` - `CoreApi` (extends `CoreClient`) - typed convenience methods and their request/response classes.
- `test/*.dart` - unit tests against a real local `dart:io` `HttpServer` (`package:test`, run via `dart test`), covering the same ground as the Go and TypeScript SDKs' suites.

## Building and testing

```sh
dart pub get
dart analyze
dart test
```

## Live validation

A throwaway `bin/livecheck.dart` (not committed, the same pattern every phase's manual verification in this repo has used) exercised the whole SDK against real running Keycloak/core-api/realtime-gateway: minted a real password-grant token, called every typed operation, registered/listed/revoked a real device, paginated every real application via `paginate`, confirmed a nonexistent user decodes into a real `RESOURCE_NOT_FOUND` `ApiError`, and opened two real WebSocket connections that both subscribed to the same channel and confirmed a genuine cross-connection fan-out. `apps/mobile`'s `flutter build macos` was not exercised - no platform runner (iOS/Android/macOS project scaffolding) exists yet for that app, a pre-existing gap from Phase 9 unrelated to this SDK; `flutter analyze` and `flutter test` (real widget behavior, mocked network per Flutter's own testing constraints) both pass clean.
