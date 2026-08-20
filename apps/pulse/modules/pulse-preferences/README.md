# pulse-preferences

Pulse-side preference surface: notification detail level (Detailed / Private / Silent, spec §72), device delivery policy, and a Pulse UI over Core's real `notifications.QuietHours` (scoped by Pulse's own `AppID` - the values themselves are Core's, only the Pulse-specific preference shape is owned here).

**Owns:** `pulse_preferences` table (notification detail level, mute state per spec §34 - distinct from Block).
**Depends on:** Core `notifications` (QuietHours, delivery preferences), `pulse-profile`.
**Status:** planned - Phase 13 (Quiet Hours / Experience Controls). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
