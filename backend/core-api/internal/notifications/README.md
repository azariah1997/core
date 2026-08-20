# Notification Platform

The platform's single entry point for sending a notification to a user - "applications should call the Core Notification Service, not FCM/APNs directly" is the roadmap's own framing. `Service.Send` is that entry point; channel dispatch (push/email/SMS/in-app/realtime) is an internal implementation detail no caller ever reaches around.

## Responsibilities

- Own `Notification` (the logical, potentially multi-channel request), `NotificationTemplate`, `NotificationPreference` (per-category opt-outs), `QuietHours`, and `NotificationDelivery` (one channel's attempt/outcome) - the four constructs plus quiet-hours/opt-out support the roadmap names.
- `Send` resolves title/body (literal or rendered from a `NotificationTemplate` via Go's `text/template`), writes the `Notification` durably, then per requested channel: skip if the user has explicitly disabled that channel for that category (`NotificationPreference`), defer if the channel is interruptive (`push`/`sms`) and quiet hours are active, or dispatch to the channel's `ChannelSender` and record the outcome.
- Emit `notification.requested` (on send), and `notification.sent`/`notification.delivered`/`notification.failed` (per delivery outcome) via the transactional outbox - the four events the roadmap names. `deferred`/`skipped` outcomes intentionally emit nothing; those aren't in the roadmap's event list.
- `RetryDelivery` re-invokes a channel's sender for a delivery that previously failed or was deferred.

## Scoping decisions

- **No real push/email/SMS provider.** The roadmap's own Cloud Rule requires local development to remain possible without paying for production infrastructure, and unlike Postgres/Redis/Keycloak/OpenFGA, push/email/SMS providers have no honest local-equivalent server. `senders.LogSender` is the local/dev stand-in for APNs/FCM/SES/an SMS gateway - it logs what would have been sent and reports success, never pretending to reach a real device/inbox/phone. A real provider client would implement the same `ChannelSender` interface later without touching anything else.
- **Push is still a genuine, live-testable integration**, despite no real APNs/FCM account: `senders.PushSender` checks whether the recipient has an actual registered device with a push token (Phase 5) before "sending." A user with zero devices gets a real failure, not a simulated one, and registering a device then calling `RetryDelivery` genuinely succeeds afterward - verified live during this phase's validation pass.
- **realtime and in-app need no external provider at all.** `senders.RealtimeSender` publishes over the same `platformkit/rtbus` bus `messaging.Service` uses (Phase 11) - a connected client receives it on its WebSocket with zero action required. `senders.InAppSender`'s "delivery" is trivial by design: the `Notification` row, already durably written before any sender runs, *is* the in-app inbox entry (`GET /v1/notifications`); the sender exists only so `in_app` produces a `NotificationDelivery` row shaped like every other channel's.
- **Send allows any authenticated caller to notify any user**, attributed via `SentByUserID` for audit - relaxed from the original self-or-platform.admin restriction once a real product (Pulse's Phase 5 push fallback: the sender notifying the receiver) turned out to be the first thing this restriction had ever actually blocked. Grepping every internal caller of `Send` across this whole platform at the time found zero cross-user callers - not even `messaging`, which delivers to other users constantly, ever went through `Send`; it only ever used realtime delivery directly, with no push fallback of its own. That confirmed self-or-admin wasn't protecting a real workflow, just blocking the one this platform's first real cross-user notifier needed. The receiver's own `NotificationPreference`/`QuietHours` are still enforced exactly as before regardless of who the sender is - loosening who may *trigger* a send never loosened what the recipient actually *receives*.
- **Template management is platform.admin only.** Templates are cross-cutting app configuration, not a per-user action - `AdminChecker` is satisfied directly by `*authz.Service` (identical method signature to `IsPlatformAdmin`, no adapter needed).
- **Retries have no background scheduler.** `RetryDelivery` is a callable Service method (and a REST endpoint scoped to the recipient) rather than something a cron/worker fires automatically - consistent with the rest of this platform, where the outbox-to-Kafka relay worker doesn't exist yet either.
- **One title/body per template, not per channel.** A template renders the same text for every channel a send targets; per-channel variants (e.g. a shorter SMS body) are a deliberate scope-down, deferred until a real need shows up.

## Layout

- `domain.go` - types, validation, `Repository` interface, `QuietHours.Active()`.
- `service.go` - `Service`, the `ChannelSender`/`AdminChecker` interfaces it depends on, and `Send`'s skip/defer/dispatch logic.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `senders/` - `ChannelSender` implementations: `PushSender` (real device-token precondition, logged send), `LogSender` (email/SMS local stand-in), `RealtimeSender` (real, via `rtbus`), `InAppSender` (trivial, real by construction).
- `postgres/` - the production `Repository`. See the naming note below.
- `memory/` - in-memory `Repository` for tests.

## Storage

The pre-existing scaffold table `notifications` turned out to already be shaped as a single-channel delivery attempt (`category`, `channel`, `payload`, `status`, `delivered_at`) - not the logical, potentially multi-channel request a caller sends. Rather than rename it, it became `NotificationDelivery`'s table (extended via `0011_notifications.sql` with `notification_request_id`, `provider_ref`, `error`, `attempts`, `updated_at`), and the logical request got a new table, `notification_requests`. `notification_templates`, `notification_preferences`, and `notification_quiet_hours` are also new in `0011_notifications.sql`.
