# pulse-connections

Pulse-side Friend / Close-Friend classification and permission checks layered over Core's generic relationship graph. The underlying connection is a Core `Relationship{Type: "pulse_friend", ...}`; "close friend" and any Pulse-specific permission flags are Pulse-owned metadata keyed by `relationship_id`, kept out of Core's product-agnostic relationship row.

**Owns:** `pulse_connection_classifications` table - the Friend/Close-Friend overlay only. The connection lifecycle itself (request/accept/decline/remove) is never duplicated; every call forwards to Core's real relationships API scoped to relationship type `pulse_friend`.
**Depends on:** Core `relationships` (connection lifecycle: request/accept/decline/remove/block), Core `groups` (Circles, not yet wired), `pulse-profile`.
**Status:** implemented - Phase 2 (Connection Experience), live-validated end to end with two real Keycloak users: request → accept → asymmetric close-friend classification → duplicate-request conflict correctly forwarded from Core → remove (soft transition to `ended`, matching Core's real semantics). Real source in `apps/pulse/api/internal/pulseconnections/`.
