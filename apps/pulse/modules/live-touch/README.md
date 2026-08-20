# live-touch

Flagship synchronous two-way touch feature (spec §21): session invite/accept, both participants on a real-time channel, touch-start/touch-stop routed bidirectionally with minimal latency. Session lifecycle (invite/accept/timeout/end) is a durable REST flow; the touch events themselves are ephemeral realtime-only messages, never persisted per-event.

**Owns:** `live_touch_sessions` table (lifecycle only - not individual touch events).
**Depends on:** `bond` (Live Touch is Bond-gated per spec §7), `pulse-connections`, `pulse-interactions` (completion is recorded as an interaction for Moments).
**Realtime:** a per-session channel (`pulse:live-touch:{sessionId}`) via `realtime-gateway`'s `PublishToChannel`.
**Status:** planned - Phase 10 (Live Touch). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
