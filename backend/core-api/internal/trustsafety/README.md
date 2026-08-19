# Trust & Safety

Phase 21's own list: Block, Mute, Report, ModerationCase, Suspension, Ban, Appeal, plus rate limiting, spam protection, and abuse signals.

## Block already exists - not duplicated

Phase 8's `relationships` module already is the platform's reusable block primitive (`Relationship.Status = "blocked"`, `POST /v1/relationships/block`). This package doesn't reimplement it - Mute, the one genuinely new "hide this person" primitive, is deliberately different in kind: one-directional and silent (the muted user is never notified and isn't prevented from interacting - only the muter stops seeing them), unlike Block, which is a full relationship state change both sides can observe.

## Report -> ModerationCase, deduplicated

"Design so products can supply product-specific report reasons" is satisfied the same way `RelationshipType` (Phase 8) and `Message.Type` (Phase 11) are: `Report.Reason` is a free-form string, never a fixed platform enum.

Every report funnels into a `ModerationCase`, one per `(ResourceType, ResourceID)` while a case stays open - `OpenOrAttachCase` uses `INSERT ... ON CONFLICT ... DO UPDATE` against a partial unique index (`WHERE status IN ('open','in_review')`) to atomically create-or-attach in one statement, so ten reports about the same message become one case with `ReportCount` 10, not ten separate cases for a moderator to deduplicate by hand - confirmed live: two different users reporting the same message both landed on the identical case ID, with `reportCount` incrementing correctly. A resource that already had a *resolved* case can still get a fresh one later - renewed abuse deserves a new look, not silence because an old case is closed.

## Suspension and Ban actually restrict access - in one place

The roadmap's "reusable Suspension, Ban" only means something if they actually restrict something. Rather than retrofitting an "am I restricted" check into every one of the platform's ~20 other modules, `Service.IsRestricted` is checked in exactly one place: a new `requireActive` middleware (`internal/api/session.go`), layered on top of every other module's existing `requireUser`. This is the same "one entry point, not per-module logic" principle `authz.Service.Can` already establishes for permissions - a `403` with the actual reason (`"account is restricted: suspended: repeated spam reports"`), confirmed live against three different already-gated routes for a suspended user.

**Two deliberate exemptions**, wired with plain `requireUser` instead of `requireActive` in `router.go`: `users` (so a restricted caller can still see their own profile and status) and `trustsafety` itself (so they can still file an appeal). A restricted user must never be locked out of knowing why, or contesting it - confirmed live: a suspended user's own token was rejected by `/v1/users/me/devices`, `/v1/jobs`, and `POST /v1/relationships`, but still succeeded against `/v1/users/me` and `/v1/trustsafety/appeals`.

`IsActive` is computed at query time (`Suspension`: not lifted and before `EndsAt`; `Ban`: not lifted) rather than stored as a mutable flag a scheduler would need to flip - the same "don't invent scheduler infrastructure this phase doesn't need" principle as Phase 13's `PurgeExpired` and Phase 20's retention policies.

## Appeal

A suspended or banned user can appeal - `CreateAppeal` verifies the caller actually owns the target restriction (a stranger can't appeal someone else's ban, confirmed live: 403). `ReviewAppeal` is the one place approving an appeal has a real side effect: it lifts the appealed `Suspension`/`Ban` in the same call, confirmed live end-to-end - a suspended user's blocked route became reachable again immediately after their appeal was approved, with no separate "now go lift it" step for the moderator to forget. Reviewing an already-reviewed appeal correctly 409s (confirmed live), rather than silently allowing a second, contradictory decision.

## Rate limiting and spam protection

A new shared package, `packages/go/platformkit/ratelimit` (Valkey/Redis-backed fixed-window counter, `INCR`+`EXPIRE`), lives in `platformkit` alongside `rtbus`/`searchidx` for the same reason those do - cross-cutting infrastructure any service might need, not something specific to this module. This phase wires it to exactly one real, well-motivated integration: `CreateReport` is limited to 5 reports/hour per reporter, confirmed live against real Valkey (the 5th and 6th rapid report both correctly `429`). Retrofitting a limiter onto every write in this package would have been speculative; report-spam is the one action here a bad actor would most want to abuse.

## Abuse signals - a deliberately simple escalation rule

`AbuseSignal` is a lightweight, open-to-record observation - distinct from `Report` (a person reporting something) and from Phase 19's audit (a record of what happened): a signal is "something looks off about X," which may or may not ever need a human to look at it. The one escalation rule this phase implements: a single `critical`-severity signal immediately opens (or attaches to) a `ModerationCase`; anything lower does not - confirmed live, a `low` signal against a resource produced zero cases, a subsequent `critical` signal against the same resource produced exactly one. Aggregating lower-severity signals over a time window into an escalation is real abuse-detection-engine territory, deliberately out of this phase's scope.

## Moderator access

`authz.Service` gains `IsModerator` this phase, alongside its existing `IsPlatformAdmin` - sourced from RBAC (`HasRole(ctx, userID, RoleModerator)`) directly, not the fine-grained OpenFGA relation, because `AssignRole` only ever writes an OpenFGA grant for `platform.admin` specifically (see `authz/service.go`). `trustsafety.ModeratorChecker` is satisfied directly by `*authz.Service`, the same "no adapter needed" pattern as every other module's `AdminChecker`.

## Layout

- `domain.go` - `Mute`, `Report`, `ModerationCase`, `Suspension`, `Ban`, `Appeal`, `AbuseSignal`, `Repository`.
- `service.go` - `Service`, the moderator/rate-limiter dependencies, `IsRestricted` (the middleware hook).
- `http.go` - the REST surface, 18 routes.
- `postgres/`, `memory/` - `Repository` implementations; `OpenOrAttachCase` is implemented in both (an `INSERT ... ON CONFLICT` in Postgres, a linear scan under a mutex in memory).

## Storage

`mutes`, `moderation_cases`, `reports`, `suspensions`, `bans`, `appeals`, `abuse_signals` (`data/migrations/0019_trustsafety.sql`) - all new tables, no pre-existing scaffold.

## Not done here

No outbox event is emitted anywhere in this package, matching Phase 19 (audit) and Phase 20 (privacy) rather than Phase 14/18's event-driven pattern - moderation actions are read back directly via this module's own API, not consumed asynchronously by anything yet; `contracts/asyncapi/events.yaml` is unchanged this phase, confirmed rather than silently skipped. Enforcement of Suspension/Ban is also deliberately singular: only HTTP access is gated (`requireActive`); a suspended user's existing WebSocket connection to `realtime-gateway` (Phase 10, a separate service) is not forcibly disconnected - a real follow-up, not solved here.
