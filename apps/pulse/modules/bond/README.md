# bond

The one-active-romantic-Bond product policy (spec §7): request/accept/decline/end, enforcing "at most one ACTIVE bond per user" - a genuine concurrency-sensitive invariant (two simultaneous accepts must not both win), not embedded into Core since it's Pulse product policy, not a platform concept.

**Owns:** `bonds` table (references `relationships.id`; a Postgres partial unique index `WHERE status = 'ACTIVE'` is the real enforcement mechanism, not just an application-level check - see Risk #2 in the architecture audit).
**Depends on:** Core `relationships` (the underlying connection), `pulse-connections`.
**Status:** planned - Phase 3 (Partner Bond). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
