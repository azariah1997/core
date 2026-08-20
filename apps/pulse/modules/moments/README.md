# moments

Private saved-highlights timeline of meaningful interactions (spec §30-31) - explicitly not a chat history, no message content, just references to interactions plus participants/timestamp/duration. Stores references, never duplicates interaction payload.

**Owns:** `moments` table (references `interactions.id`, never duplicates its content).
**Depends on:** `pulse-interactions`.
**Status:** planned - Phase 12 (Moments). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
