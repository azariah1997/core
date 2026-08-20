# pulse-connections

Pulse-side Friend / Close-Friend classification and permission checks layered over Core's generic relationship graph. The underlying connection is a Core `Relationship{Type: "pulse_friend", ...}`; "close friend" and any Pulse-specific permission flags are Pulse-owned metadata keyed by `relationship_id`, kept out of Core's product-agnostic relationship row.

**Owns:** Pulse-side connection permission/classification table.
**Depends on:** Core `relationships` (connection lifecycle: request/accept/decline/remove/block), Core `groups` (Circles), `pulse-profile`.
**Status:** planned - Phase 2 (Connection Experience). No code yet; see `apps/pulse/docs/ARCHITECTURE_AUDIT.md`.
