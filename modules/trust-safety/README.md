# Trust Safety module

Owns the **trust-safety** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/trustsafety` - `Mute`/`Report`->`ModerationCase` (deduplicated via a partial unique index)/`Suspension`/`Ban`/`Appeal`. Blocking deliberately reuses the `social` module's relationships (`Status: blocked`) rather than duplicating it. A new `requireActive` HTTP middleware (`internal/api/session.go`) makes an active Suspension or Ban actually restrict platform access from one place, instead of every module re-checking independently. Report spam is rate-limited via `platformkit/ratelimit` (Valkey-backed); a critical-severity `AbuseSignal` auto-opens a case. Exposed at `/v1/trustsafety/*`. See that package's README for detail.
