# mood

Today's Mood: passive, wordless daily emotional broadcast. Visual selection (not free text - spec §22-23), per-mood audience (`PARTNER_ONLY`/`CLOSE_FRIENDS`/`SELECTED_CIRCLES`/`ALL_CONNECTIONS`/`CUSTOM_USERS`/`PRIVATE`), day-boundary/timezone-correct expiry (spec §26).

**Owns:** `moods` table.
**Depends on:** `pulse-connections` (audience resolution against Friend/Close-Friend classification), Core `groups` (Circle-scoped audience).
**Status:** planned - Phase 8 (Today's Mood). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
