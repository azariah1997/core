# signals

Private custom touch-pattern storage (spec §19-20): the "private touch language" differentiator - a bounded sequence of tap/hold/pause segments whose meaning exists only between two people. The platform never interprets semantic meaning, only stores and replays the pattern.

**Owns:** `custom_signals` table. Segment count and total duration are server-enforced bounds (Risk #4 in the architecture audit) - an unbounded pattern is a real abuse vector (very long/rapid vibration).
**Depends on:** `pulse-connections`, `bond` (patterns are typically relationship-scoped, not global).
**Status:** planned - Phase 11 (Custom Touch Language). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
