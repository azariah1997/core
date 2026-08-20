# Pulse Integration & Architecture Audit

Performed per `PRODUCT_SPEC.md` §106, before any Pulse feature code was written. This inspects the real, currently-running Core Platform (all 30 roadmap phases complete, see the root `VALIDATION.md`) and maps it onto what Pulse needs - not what a generic platform might have. Every capability named below was confirmed by reading the actual source in this repository, not assumed from the module name.

## CORE CAPABILITIES REUSED

| Pulse need | Core capability | Real source |
|---|---|---|
| Auth (§53) | Keycloak-backed identity, JWT validation | `backend/core-api/internal/identity` |
| User model (§54) | Platform `User`, auto-provisioned on first login | `backend/core-api/internal/users` |
| Multi-device / push tokens (§73) | `Device{Platform, HasPushToken}`, revoke/reactivate | `backend/core-api/internal/devices` |
| Connections, Friend/Close-Friend/Bond (§6-11, §55) | Generic `Relationship{RequesterID, TargetID, Type, Status}` - `Type` is a free product-defined string, `Status` is exactly `pending/active/blocked/ended` (matches Bond's own required states almost verbatim) | `backend/core-api/internal/relationships` |
| Circles (§10) | Generic `Group`/`GroupMember`, free-form member roles | `backend/core-api/internal/groups` |
| Authorization (§89) | `authz.Service.Can`/`IsPlatformAdmin`, RBAC + OpenFGA ReBAC | `backend/core-api/internal/authz` |
| Blocking (§35) enforcement | `relationships.StatusBlocked` (connection-level) + `trustsafety.Ban`/`requireActive` middleware (platform-wide restriction) | `backend/core-api/internal/relationships`, `backend/core-api/internal/trustsafety` |
| Muting (§34) | `trustsafety.Mute{MutedUserID}`, distinct from Block per the spec's own requirement | `backend/core-api/internal/trustsafety` |
| Reporting/abuse (§36, §38) | `Report → ModerationCase` (deduplicated), `AbuseSignal` with severity-based auto-escalation | `backend/core-api/internal/trustsafety` |
| Rate limiting (§37) | `ratelimit.Limiter.Allow(ctx, key, limit, window)`, Valkey-backed, already used by `trustsafety` for report spam | `packages/go/platformkit/ratelimit` |
| Realtime delivery (§15, §61, §74) | `hub.PublishToUser` (1:1, exactly `PulseStart`/`PulseStop` targeted delivery) and `hub.PublishToChannel` (session fan-out, exactly Live Touch) over a real hand-rolled WebSocket, Redis-fanned across replicas | `backend/realtime-gateway/internal/hub`, `internal/ws` |
| Push notifications (§71-72) | `notifications.Service.Send`, channel-neutral (`push/email/sms/in_app/realtime`), never calls FCM/APNs directly from a caller | `backend/core-api/internal/notifications` |
| Quiet Hours (§33) | `QuietHours{user, appID}`, already distinguishes interruptive channels (push/SMS) from passive ones (in_app/realtime) - exactly the spec's §33 semantics, not something to build | `backend/core-api/internal/notifications` (`domain.go` `QuietHours`, `Interruptive()`) |
| Delivery retry/quiet-hours-aware routing (§74) | `notifications.StatusDeferred`/`StatusSkipped`, already real | `backend/core-api/internal/notifications` |
| Background jobs (scheduled work) | Immediate/scheduled/delayed/recurring job runner | `backend/core-api/internal/jobs` |
| Scheduled Pulse (§41) | Temporal-backed `workflows.Service.Start(..., cronSchedule, ...)` | `backend/core-api/internal/workflows` |
| Feature flags (§92) | `Feature`/`FeatureRule` targeting app/env/user/tenant/%/version | `backend/core-api/internal/features` |
| Remote configuration (§93) | Typed key/value scoped by `(AppID, Environment, Key)`, full change-audit | `backend/core-api/internal/remoteconfig` |
| Entitlements (§84) | `Entitlement.HasEntitlement(userID, key)`, provider-neutral | `backend/core-api/internal/billing` |
| Analytics (§79-80) | Generic event envelope, unauthenticated ingest, worker-batched to a data lake | `backend/core-api/internal/analytics` |
| Privacy/export/delete (§39-40) | Consent, preferences, cross-module export/delete registry | `backend/core-api/internal/privacy` |
| Audit (§95 debugging trail) | Immutable actor/action/resource/correlation record | `backend/core-api/internal/audit` |
| Observability (§95-97) | `/metrics` (Prometheus), structured logs with `trace_id` correlation (Loki), distributed traces (Tempo), `X-Correlation-ID` propagation | `packages/go/platformkit/{metrics,logging,otelx,correlation}` |
| Application registry | `applications.Application` - Pulse registers itself here to get an `AppID`, which every scoped capability above (`QuietHours`, `remoteconfig`, `relationships`) keys off | `backend/core-api/internal/applications` |
| Client SDK (§53, no direct HTTP) | Go/TypeScript/Dart SDKs with auth/refresh/pagination/retries/realtime client, a real Flutter consumer already exists (`apps/mobile`) | `packages/flutter/core_sdk` |
| Product-specific admin (§91) | Admin Portal pattern (`apps/admin`) - real per-module panels reading live core-api data | `apps/admin` |
| Catalog / architecture visibility (§98) | Backstage catalog, machine-checked `dependsOn` graph | `catalog/system.yaml` |

## PULSE-SPECIFIC CAPABILITIES

Nothing above models a *pulse* as a felt, timed, non-verbal gesture, a *Bond* as a distinct product concept, a *Mood* as a passive daily emotional broadcast, or a *haptic engine*. These are genuinely new and belong entirely inside Pulse:

- **`pulse-profile`** - `PulseProfile{userID, publicHandle, visualPrefs, pulsePrefs}`, a thin extension row, never a duplicate of Core's `User`.
- **`pulse-connections`** - Pulse-side UI/permission logic over Core relationships (friend vs. close-friend classification is Pulse policy, not a new Core relationship type - it can be `relationships.Type = "pulse_friend"` with a Pulse-owned `closeFriend: true` metadata flag, or a Pulse-owned side table; decided in "Data ownership" below).
- **`bond`** - the one-active-Bond policy, Bond-specific permission gates (Live Touch, private touch language). Uses `relationships.Type = "pulse_bond"` for the underlying connection but owns the "only one ACTIVE bond at a time" invariant, which is Pulse product policy per §7, not a Core concept.
- **`mood`** - Today's Mood: visual selection, per-mood audience, day-boundary/timezone expiry. No Core module models transient daily emotional broadcast.
- **`pulse-interactions`** - the `Pulse`/`Knock`/`MoodResponse` interaction records, state machine (§16), duration validation, idempotency.
- **`signals`** - custom touch-pattern storage and playback (§19-20); Core has no concept of a tap/hold/pause pattern.
- **`live-touch`** - synchronous two-way touch sessions layered on Core's realtime channels; the session lifecycle (invite/accept/timeout) is Pulse's.
- **`moments`** - the private saved-highlights timeline; explicitly "no chat" per §30/§48, distinct from Core's `messaging` module which is a full conversation primitive Pulse deliberately does not use.
- **`pulse-preferences`** - Quiet Hours *values* live in Core's `notifications.QuietHours` (scoped by Pulse's `AppID`); Pulse-specific preference *shape* (e.g. notification detail level: Detailed/Private/Silent per §72) is Pulse-owned.
- **`pulse-entitlements`** - Pulse plan tiers (`pulse.plus`) as Core `Entitlement` keys; the mapping of which Pulse features each key unlocks is Pulse-owned.
- **`HapticEngine`** (mobile-only, §59-60) - `playPulseStart/Stop/Pattern/Knock/MoodResponse`, `IOSHapticEngine`/`AndroidHapticEngine` adapters, capability detection. Purely a mobile-app concern; no backend involvement.

## CORE GAPS

Only two genuine gaps found - everything else the spec assumes Core should provide already exists and was verified against real source, not assumed:

1. **Deep linking (§52).** No deep-link infrastructure exists anywhere in Core (`grep -ri deep.?link` across `backend/`, `packages/`, `modules/` returns nothing). This is arguably not a *generic platform* capability to begin with - it's Flutter `app_links`/iOS Universal Links/Android App Links configuration that lives entirely in the mobile app's own platform projects, plus a thin `POST /v1/pulse/invites` endpoint Pulse itself owns to resolve an invite code. **Recommendation: build this in Pulse, not Core** - a second product needing deep links later would want its own link scheme anyway, so this doesn't meet the "genuinely generic, benefits all future apps" bar `AI_CONTEXT.md` sets for changing Core.
2. **Multi-device delivery *policy* (§73-74).** Core's `devices` module tracks devices and push tokens; `notifications.Service.Send` can target a user, but nothing in Core currently expresses "send to all active devices" vs. "primary device only" vs. "wearable preferred" as a per-caller policy - `notifications` sends to whatever devices `devices.Service` returns for that user, all of them. **Recommendation: defer.** Pulse's Phase 4/5 (Basic Pulse, Push Fallback) can launch correctly against Core's current "all active devices" behavior; a policy layer only matters once Pulse has more than one device type in practice (e.g. once wearables, §17, are real). Revisit as a genuine Core enhancement at that point, not now - it would benefit any future multi-device product, so it's a legitimate future Core change, just not a Phase 1-10 blocker.

No other gap was found. In particular, Quiet Hours, blocking, muting, rate limiting, scheduled delivery, and entitlements - each explicitly named in the spec as something to "use Core X" - are all real and already load-bearing in production code today (e.g. `trustsafety` already uses `ratelimit` for report spam; Pulse's Knock/Pulse rate limiting is the same call with a different key).

## DATA OWNERSHIP

**Core owns** (Pulse never duplicates): `User` identity, `Device` records, the base `Relationship` row (requester/target/type/status/metadata), `Group`/`GroupMember`, role/permission grants, `QuietHours` values, `Entitlement` grant records, `NotificationDelivery` records, `AuditRecord`s, analytics event storage.

**Pulse owns** (new tables, in `apps/pulse/api`'s own Postgres schema - never inside `backend/core-api`'s database): `pulse_profiles`, `bonds` (the one-active-bond invariant + Bond-specific metadata, referencing `relationships.id`), `moods`, `interactions` (Pulse/Knock/MoodResponse/LiveTouch, the state machine from §16), `pulses` (1:1 with an `interaction` of type PULSE, duration/delivery_mode), `moments`, `custom_signals`, `pulse_preferences`. Every Pulse table references a Core `user_id`/`relationship_id` by value (never a foreign key across service/database boundaries - Pulse's Postgres instance is logically separate from Core's, consistent with `ADR-002-domain-data-api`'s "domain APIs hide storage vendors and physical schemas").

Relationship metadata split: the base `relationships` row (who/whom/type/status) is Core's; Pulse-specific relationship *permissions* (e.g., can this Friend receive Live Touch invites?) live in Pulse's own `pulse_connections`-adjacent table keyed by `relationship_id`, not stuffed into Core's generic `Metadata map[string]any` field - keeping Core's relationship row product-agnostic per `AI_CONTEXT.md`'s non-negotiable rule.

## MODULE BOUNDARIES

Ten Pulse modules, mirroring Core's own `domain.go`/`service.go`/`http.go`/`postgres`/`memory` layered pattern exactly (see `AI_CONTEXT.md`), each independently testable against an in-memory fake:

```
pulse-profile      - PulseProfile CRUD, extension of Core User
pulse-connections  - Friend/Close-Friend classification and Pulse-specific permission checks over Core relationships
bond               - one-active-Bond invariant, Bond request/accept/decline/end
mood               - Today's Mood: set/clear/expire, audience resolution
pulse-interactions - Pulse/Knock/MoodResponse state machine, duration validation, idempotency
signals            - custom touch-pattern CRUD (bounded pattern length/duration)
live-touch         - session invite/accept/timeout, realtime channel provisioning
moments            - saved-moment CRUD, retention policy
pulse-preferences  - notification detail level, Pulse-side Quiet Hours UI over Core's QuietHours
pulse-entitlements - pulse.plus feature gating over Core Entitlement
```

Each module depends only on Core (via `coresdk`) and, where genuinely necessary, on one other Pulse module (e.g. `live-touch` depends on `bond`+`pulse-connections` to authorize a session; `pulse-interactions` depends on `pulse-connections` to check blocking/muting before creating an interaction) - never the reverse, and never a circular dependency between two Pulse modules. `bond`/`mood`/`pulse-interactions` never import each other's `postgres` packages, only their `domain.go` interfaces, matching Core's consumer-defined-interface convention.

## API CONTRACTS

Pulse owns its own OpenAPI document (`apps/pulse/contracts/openapi/pulse-api.yaml`), separate from `contracts/openapi/core-api.yaml` - Pulse's API is not a Core API, it's a product API that happens to run on the same infrastructure. Endpoint shapes as sketched in §63, refined to match Core's actual REST conventions (module-prefixed operationIds, cursor pagination `{items, nextCursor}`, the shared `apperr` error envelope):

```
POST   /v1/pulse/connections/requests
POST   /v1/pulse/connections/requests/{id}/accept
POST   /v1/pulse/connections/requests/{id}/decline
DELETE /v1/pulse/connections/{id}

POST   /v1/pulse/bond/requests
POST   /v1/pulse/bond/requests/{id}/accept
POST   /v1/pulse/bond/requests/{id}/decline
DELETE /v1/pulse/bond
GET    /v1/pulse/bond

PUT    /v1/pulse/mood
DELETE /v1/pulse/mood
GET    /v1/pulse/mood/{userId}
GET    /v1/pulse/mood/feed

POST   /v1/pulse/interactions            (creates a CREATED-state interaction; body: type, receiverId)
POST   /v1/pulse/interactions/{id}/start (PulseStart, live path)
POST   /v1/pulse/interactions/{id}/stop  (PulseStop, live path)
POST   /v1/pulse/interactions/{id}/complete (durable completion, offline path)
GET    /v1/pulse/interactions/{id}
GET    /v1/pulse/interactions            (mine, cursor-paginated)

POST   /v1/pulse/knocks

POST   /v1/pulse/signals
GET    /v1/pulse/signals
DELETE /v1/pulse/signals/{id}

POST   /v1/pulse/live-touch/sessions
POST   /v1/pulse/live-touch/sessions/{id}/accept
POST   /v1/pulse/live-touch/sessions/{id}/end

GET    /v1/pulse/moments
POST   /v1/pulse/moments/{interactionId}/save

GET    /v1/pulse/preferences
PUT    /v1/pulse/preferences
```

Auth: every route requires an authenticated Core caller (`requireUser` middleware identical to Core's, reading a Core-issued JWT); Pulse's own `http.go` resolves `callerID` from `users.FromContext` exactly like every Core module.

## REALTIME CONTRACTS

Reuses `realtime-gateway`'s existing channel/message primitives rather than inventing new wire framing (per §57's "do not create a separate realtime infrastructure"):

- **Pulse delivery** - `hub.PublishToUser(receiverID, ...)`, message `type: "pulse.started"` / `"pulse.stopped"`, payload `{interactionId, senderId, startedAt}` / `{interactionId, endedAt}`. Matches §15's exact START/STOP framing - never a continuous stream.
- **Knock** - `hub.PublishToUser`, `type: "knock.sent"`, payload `{interactionId, senderId, pattern}`.
- **Live Touch** - a per-session channel (`pulse:live-touch:{sessionId}`), both participants `Subscribe`; `type: "live_touch.touch_started"`/`"live_touch.touch_stopped"` published via `PublishToChannel`. Session invite/accept/end are durable REST calls (§75 - "critical path: sender → gateway → receiver", persistence/analytics async), not realtime messages.
- **Custom signal playback** - `type: "signal.segment"`, compact `{type, durationMs}` array per §20, delivered the same way as Pulse start/stop, never per-millisecond ticks.
- **Presence** - reuses `realtime-gateway`'s existing TTL-based presence; Pulse-specific state (`AVAILABLE_FOR_TOUCH`, `IN_LIVE_TOUCH`) is carried as a presence *value*, not a new presence system.

## EVENT CONTRACTS

Pulse owns its own AsyncAPI document (`apps/pulse/contracts/asyncapi/pulse-events.yaml`), published through Core's existing outbox mechanism (`outbox.Record`, the same durable-event pattern every Core module uses - never a second event bus):

```
pulse.completed        { interactionId, senderId, receiverId, durationMs, deliveryMode }
pulse.delivered        { interactionId, deliveredAt, deliveryMode }
knock.sent              { interactionId, senderId, receiverId, pattern }
bond.requested          { bondId, requesterId, targetId }
bond.accepted           { bondId }
bond.ended              { bondId, endedBy }
mood.updated            { userId, moodVisualId, audienceType, expiresAt }
mood.expired            { userId, moodVisualId }
moment.saved            { momentId, interactionId, participantIds }
signal.created          { signalId, ownerId }
interaction.reported    { interactionId, reporterId, reason }
```

Ephemeral realtime events (pulse.started/stopped at the WS layer, live_touch.touch_started/stopped, signal.segment) are deliberately **not** written to the outbox - only their *durable* counterpart (`pulse.completed`, not every start/stop) is, matching §61/§90's "ephemeral events should remain ephemeral" and Core's own established pattern (Phase 26's audit found most modules write to the outbox only on durable state transitions, not every intermediate step).

## AUTHORIZATION MODEL

Every Pulse interaction is authorized server-side before it does anything, per §89 - never trusted from the client:

1. **Caller identity** - resolved from the Core JWT via `users.FromContext`, identical to every Core module.
2. **Connection exists and is active** - `relationships.Service` lookup between sender and receiver; a Pulse/Knock/Live-Touch to a non-connected user is rejected (`ErrForbidden`).
3. **Not blocked** - `relationships.StatusBlocked` check both directions, plus `trustsafety`'s platform-wide ban/suspension check via the same `requireActive`-style gate Core already uses.
4. **Bond-gated features** - Live Touch to a non-Bond target, or private touch-language patterns, additionally require `bond.Service` to confirm an `ACTIVE` bond between the two users.
5. **Mood audience** - a Mood read checks the viewer against the Mood's configured audience (`PARTNER_ONLY`/`CLOSE_FRIENDS`/`SELECTED_CIRCLES`/`ALL_CONNECTIONS`/`CUSTOM_USERS`/`PRIVATE`), resolved via `pulse-connections` classification plus `groups.Service` for circle membership - never assumed all connections see all Moods (§25).
6. **Rate limits** - `ratelimit.Limiter.Allow` keyed per `(senderID, interactionType, window)` and per `(receiverID, window)` independently, both remote-configurable via `remoteconfig` (§37 - "never hardcode one global limit").
7. **Mute** - checked at delivery time (does the receiver mute the sender?), distinct from the above authorization gates - a muted-but-not-blocked sender's Pulse still succeeds and is recorded, but delivery is suppressed/deferred per §34's "mute behaviour must be distinct from Block."

## MOBILE ARCHITECTURE

Flutter, consistent with Core's existing `apps/mobile` (§58). New `apps/pulse/mobile`:

- **`core_sdk`** (`packages/flutter/core_sdk`, already real) provides auth/token-refresh, the realtime WS client, and device registration - Pulse's mobile app never talks to Keycloak or the realtime gateway directly, exactly like `apps/mobile` today.
- **A new Pulse-specific Dart client** (either a thin hand-written wrapper over `core_sdk`'s `request()` escape hatch, or a small generated client from `pulse-api.yaml` - decided at Phase 1) calls the Pulse Product API's own REST routes.
- **`HapticEngine`** abstraction (§59-60): a Dart interface (`playPulseStart/Stop/Pattern/Knock/MoodResponse`, `capabilityLevel()`) with `IOSHapticEngine` (Swift, Core Haptics via a platform channel) and `AndroidHapticEngine` (Kotlin, `Vibrator`/`VibrationEffect` via a platform channel) adapters, plus a no-op `UnavailableHapticEngine` fallback for web/unsupported devices - consistent with §60's graceful-degradation requirement and §86's accessibility requirement that the product still work without advanced haptics.
- **Navigation shell** (§44): five tabs - Home, People, Mood, Moments, Profile - a `BottomNavigationBar`-based shell, matching the spec's explicit "avoid excessive navigation."
- **Realtime lifecycle**: the WS connection opens only while the app is foregrounded (battery discipline per §87), using `core_sdk`'s existing realtime client; background delivery always falls back to push (§4.2), never a background WS reconnect loop.

## RISKS

1. **iOS background haptic promises.** The single highest product risk named in the spec itself (§4) - any implementation that implies continuous remote vibration while backgrounded/locked on iOS will misrepresent what the OS allows. Mitigation: Live Pulse and Offline Pulse are architecturally distinct code paths from Phase 4 onward, never a "best effort" blend; `apps/mobile`'s Flutter web build already demonstrated this session that platform capability gaps (no Android SDK/Xcode here) mean iOS-specific haptic behavior cannot be end-to-end validated in this environment until a real device/simulator is available - this should be flagged honestly in every phase's validation, not glossed over.
2. **One-active-Bond concurrency (§Phase 3).** Two simultaneous Bond requests targeting the same user must not both succeed. Needs a real concurrency test (two goroutines racing `AcceptBond`) against Postgres with a unique partial index (`WHERE status = 'ACTIVE'`) as the actual enforcement mechanism, not just an application-level check - the same "don't trust a race-free assumption" lesson Core's own `billing`/`authz` modules learned.
3. **Trusting client-submitted duration (§15, §78).** Must be enforced identically to how `jobs`/`workflows` already treat client input as untrusted - server timestamps are truth, client `startedAt`/`endedAt` are UX-only. A test asserting a forged large `durationMs` in a `complete` payload is ignored in favor of server-observed `PulseStart`→`PulseStop` timing (or `PulseStart`→arrival-of-`complete` for the offline path) should exist before Phase 4 is called done.
4. **Custom signal pattern abuse (§Phase 11).** An unbounded `segments[]` array could be used to construct a very long or rapid vibration pattern. Needs explicit bounds (max segment count, max total duration) enforced server-side at creation time, not just client-side UI limits.
5. **Deep-link + invite-link growth loop (§51-52) intersects Trust & Safety.** An open invite-link flow is a natural spam vector; must reuse `trustsafety`'s rate limiting on invite creation, not just on Pulse/Knock sending.
6. **Toolchain gap for real device testing.** Confirmed this session: this environment has Flutter but no Android SDK and an incomplete Xcode install. Live validation of native haptics (the product's actual core value) cannot happen end-to-end here without that toolchain - a real, honest gap to name in every phase's validation, per the platform's real-infra-only validation philosophy, rather than simulating haptic behavior and calling it validated.

## PHASED IMPLEMENTATION PLAN

The spec's own 17 phases (§Phase 1-17) are adopted as-is - they already follow Core's own "vertical slice, independently validated" discipline and map cleanly onto the module boundaries above. No re-sequencing needed. Each phase should close with the same loop `AI_CONTEXT.md` documents for Core: Inspect → Design → Implement → Test (service-layer + handler-layer) → Validate live → Document → Commit.

## FIRST CODE CHANGE RECOMMENDED

**Phase 1 - Product Foundation**, scoped exactly as the spec names it: register Pulse as a Core `Application` (real `POST /v1/apps` call, giving Pulse a real `AppID` that `remoteconfig`/`QuietHours`/analytics scope against), scaffold the `apps/pulse/api` Go service (platformkit-based, matching `core-api`/`realtime-gateway`/`worker`'s existing bootstrap, wired to its own Postgres database/migrations directory), scaffold `apps/pulse/mobile` as a real Flutter app (analogous to this session's `apps/mobile` setup - `flutter create`, `core_sdk` path dependency, the five-tab navigation shell with placeholder screens), stand up the `pulse-profile` module end-to-end (the simplest module: `domain.go`/`service.go`/`http.go`/`postgres`/`memory`, service + handler tests, no cross-module dependencies) as the first real proof the whole chain works, and register the feature-flag/analytics namespaces (`pulse`, `pulse_back`, `knock`, `mood`, `live_touch`, `custom_signals`, `moments`, `scheduled_pulse`, `wearables`) in Core's `features` module. This is deliberately the smallest slice that proves every layer (mobile → Pulse API → Core SDK → Core API → Postgres) end-to-end before Phase 2 touches anything relationship-shaped.
