# bond

The one-active-romantic-Bond product policy (spec §7): request/accept/decline/end, enforcing "at most one ACTIVE bond per user" - a genuine concurrency-sensitive invariant (two simultaneous accepts must not both win), not embedded into Core since it's Pulse product policy, not a platform concept.

**Owns:** `bonds` table (bond-specific state) plus `bond_active_holders` (`user_id PRIMARY KEY` - one row per user, ever, regardless of which bond or side - the real one-active-bond enforcement mechanism, not an application-level check).
**Depends on:** Core `relationships` (the underlying `pulse_bond` connection; requires an existing active `pulse_friend` connection first, per product spec §11), `pulse-connections`.
**Status:** implemented - Phase 3 (Partner Bond), live-validated end to end including real concurrency: two goroutines racing `Accept` against real Postgres for the same target user, confirmed exactly one succeeds across 4 runs, the failing one receiving a real `ErrAlreadyBonded` - not an application-level race that could slip through. Real source in `apps/pulse/api/internal/bond/`.
