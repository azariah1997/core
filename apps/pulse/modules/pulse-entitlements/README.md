# pulse-entitlements

Maps Pulse premium feature gating onto Core's real `Entitlement` primitive (`hasEntitlement("pulse.plus")`, spec §84) - Pulse never talks to Stripe/Apple/Google directly, and never blocks core emotional communication (Pulse, Mood, Knock, basic Live Touch) behind a paywall per spec §83.

**Owns:** the mapping of which Pulse features each entitlement key unlocks (application-level, no new table required - Core's `Entitlement` record is the source of truth).
**Depends on:** Core `billing`.
**Status:** planned - Phase 16 (Monetisation Foundation). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
