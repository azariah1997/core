# Devices / Sessions

Per-user device tracking: which installs of which apps a user is logged in on, so they can see and revoke them (a lost phone, a shared computer, "sign out everywhere").

## Responsibilities

- `Register`: upsert by `(userID, clientDeviceID)` - a first-sight registration creates the device, a repeat one refreshes profile fields and `LastActiveAt`, and re-registering a previously revoked device reactivates it (the "log back in on this device" case). `clientDeviceID` is generated once by the client and persisted locally per install; the server never invents device identity on its own.
- `List` / `Revoke`: always scoped to a `userID`, not just a device ID, so a caller can never see or revoke another user's device even if it guessed the ID - enforced at the repository query level, not just by routing convention.
- Never expose a raw push token once registered - `Device.HasPushToken` is a boolean, there is no field capable of holding the token itself.

## Non-responsibilities

- No first-class `Session` table distinct from `Device` yet - `session_status` (`active`/`revoked`) on the device row stands in for it. Nothing in this phase needs more than one live session per device; if that changes, `Session` becomes its own resource then, not speculatively now.
- Doesn't send push notifications - `PushToken` is stored for Phase 12 (Notifications) to use, not acted on here.
- No events emitted - the platform roadmap's Phase 5 doesn't specify any, unlike Phases 2/4.

## Layout

- `domain.go` - `Device`, `Repository` interface, validation.
- `service.go` - `Service`.
- `http.go` - `POST/GET /v1/users/me/devices`, `DELETE /v1/users/me/devices/{id}`. Takes `requireUser` as a parameter (built in `internal/api`) rather than importing `identity` - same decoupling pattern as `users`. Imports `users` only for `users.FromContext` to read the caller's own resolved user ID.
- `postgres/` - the production `Repository`, upsert via `ON CONFLICT`.
- `memory/` - in-memory `Repository` for tests.

## Storage

`devices` table (`data/migrations/0001_core.sql`, `0005_devices_sessions.sql`, which adds `client_device_id`, `os_version`, `locale`, `timezone`, `session_status`, and the `(user_id, client_device_id)` uniqueness the upsert relies on).
