# pulse-interactions

The core product mechanic: Pulse (press-and-hold, felt as haptic), Knock, and Mood Response. Owns the interaction state machine (`CREATED → STARTED → LIVE_DELIVERED/PUSH_REQUESTED → COMPLETED`, spec §16), server-authoritative duration (never trusts client-submitted `durationMs` - Risk #3 in the architecture audit), and idempotent creation.

**Owns:** `interactions` and `pulses` tables.
**Depends on:** `pulse-connections` (authorization: connected, not blocked, not muted), `bond` (Bond-gated interaction types), Core `relationships` (block check), Core `notifications` (push fallback when the receiver isn't live), Core `devices` (delivery targets).
**Realtime:** delivered via `realtime-gateway`'s `PublishToUser` with `pulse.started`/`pulse.stopped` message types - see architecture audit's "Realtime Contracts".
**Status:** planned - Phase 4 (Basic Pulse), Phase 5 (Push Fallback), Phase 6 (Pulse Back), Phase 7 (Knock). No code yet.
