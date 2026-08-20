# pulse-interactions

The core product mechanic: Pulse (press-and-hold, felt as haptic), Knock, and Mood Response. Owns the interaction state machine (`CREATED → STARTED → LIVE_DELIVERED/PUSH_REQUESTED → COMPLETED`, spec §16), server-authoritative duration (never trusts client-submitted `durationMs` - Risk #3 in the architecture audit), and idempotent creation.

**Owns:** `interactions` table (idempotent on `(sender_id, client_request_id)`, server-computed `duration_ms`).
**Depends on:** Core `relationships` (connection + block check via `ListMine`, no mutation), `platformkit/rtbus` (real-time delivery, fixed service-level dependency), Core `analytics` (durable `pulse_completed` event), `platformkit/ratelimit` (10 sends/min/sender).
**Realtime:** delivered via `rtbus.Publisher.ToUser` → `realtime-gateway`'s hub → the receiver's live WebSocket, with `pulse.started`/`pulse.stopped` message types wrapped in the SDK's real `{type, data}` envelope.
**Status:** implemented (live path only) - Phase 4 (Basic Pulse), live-validated end to end with two real Keycloak users, a real WebSocket connection, real server-computed duration, and a real analytics event landing in Core's own database. Push fallback (Phase 5), Pulse Back (Phase 6), and Knock (Phase 7) reuse this same table and state machine. Real source in `apps/pulse/api/internal/pulseinteractions/`.
