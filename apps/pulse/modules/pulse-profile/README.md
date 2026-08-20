# pulse-profile

Thin Pulse-specific extension of a Core `User` - public handle, visual preferences, Pulse preferences. Never duplicates Core's identity/user record.

**Owns:** `pulse_profiles` table (`user_id` FK by value to Core's `User.ID`, never a cross-database foreign key).
**Depends on:** Core `users` (to resolve the caller and confirm the user exists).
**Status:** planned - Phase 1 (Product Foundation). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
