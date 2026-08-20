# pulse (mobile)

Pulse's Flutter app - the only UI surface for now. See `apps/pulse/docs/PRODUCT_SPEC.md` for the product, `apps/pulse/docs/ARCHITECTURE_AUDIT.md` for the architecture, and the root `docs/control/pulse.html` for build status.

## What's real (Phase 1)

- Real sign-in against Core's Keycloak realm via `core_sdk`'s `PasswordTokenSource` - Pulse never implements its own authentication.
- A five-tab navigation shell (Home / People / Mood / Moments / Profile, product spec §44) - Home/People/Mood/Moments are placeholders naming which phase builds them.
- The Profile tab is real end to end: it calls `lib/pulse_api.dart`'s thin `PulseApi` client (built on `core_sdk`'s `CoreClient`), which hits `apps/pulse/api`'s real `pulse-profile` module, which resolves the caller through Core's real `GET /v1/users/me`, and stores/returns a real row from Pulse's own Postgres database.

## Running

```bash
flutter pub get
flutter run -d chrome   # or an Android/iOS device once that toolchain is set up locally
```

Needs `core-api` (`:8080`) and `apps/pulse/api` (`:8096`) both running - see their own READMEs. Override endpoints with `--dart-define` (`KEYCLOAK_URL`, `CORE_API_URL`, `PULSE_API_URL`, `KEYCLOAK_REALM`, `KEYCLOAK_CLIENT_ID`) if not running on the default local ports.

## Testing

```bash
flutter test
```

`test/login_page_test.dart` mocks the network via `package:http/testing.dart`'s `MockClient` (Flutter's widget-test binding forces real HTTP calls to fail by design - see that file's own comment) - it proves the real widget behavior (login form, navigation, error display, the Profile tab's live data), not the SDK's HTTP logic itself (that's `packages/flutter/core_sdk`'s own `dart test` suite, against a real local server).
