# Notifications module

Owns the **notifications** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/notifications` - `Notification`/`Template`/`Preference`/`Delivery` across five channels (push, email, sms, in_app, realtime). Only push is genuinely provider-tested against a real device-token check locally; email/sms adapters are structurally supported but stubbed, the same "no real vendor credentials in this environment" reasoning later phases (billing, AI Gateway) apply explicitly. Exposed at `/v1/notifications*` and `/v1/notification-preferences*`. See that package's README for detail.
