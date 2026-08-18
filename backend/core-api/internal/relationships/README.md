# Relationships / Social Graph

A generic relationship graph. The platform never hardcodes what a relationship means - "friend", "follower", "partner" are all just product-supplied `Type` strings. What the platform owns is the generic lifecycle (`request`, `accept`, `decline`, `remove`, `block`) and enforcing it server-side.

## Responsibilities

- Own `Relationship`: `requesterId`/`targetId` (who initiated it), product-defined `type`, `status` (`pending`/`active`/`blocked`/`ended`), free-form `metadata`.
- Enforce the lifecycle server-side: only the target may accept/decline a pending request; the requester may cancel their own pending request; either participant may end an active one; either participant may block through an existing row, or block a stranger outright (creates a row directly in `blocked`).
- `Request` revives an ended relationship back to `pending` under the new requester/target rather than permanently locking that pair+type after one relationship ends - real products need "unfriend, then re-friend later" to work. Anything still live (pending/active/blocked) returns a conflict instead; the caller uses accept/decline/remove/block on the existing row.
- Emit `relationship.created` via the transactional outbox on every new row (including one created directly as `blocked`), atomically with the write.

## Non-responsibilities

- Doesn't decide what a relationship *means* to a product, or what statuses beyond the four generic ones apply - `RelationshipType` and any richer semantics are the product's, carried in `type` and `metadata`.
- No endpoints were specified anywhere for this phase (matching Phase 7/Tenants) - the endpoint set (`POST/GET /v1/relationships`, `GET/DELETE /v1/relationships/{id}`, `POST .../accept`, `POST .../decline`, `POST /v1/relationships/block`) is this package's own reasonable design built from the roadmap's five named actions.

## Layout

- `domain.go` - `Relationship`, `Status`, `Repository` interface, validation.
- `service.go` - `Service`, including all lifecycle permission checks.
- `http.go` - the REST surface. Takes `requireUser` as a parameter, same pattern as `devices`/`tenants`.
- `postgres/` - the production `Repository`. Writes into the pre-existing `relationships` table's `user_a`/`user_b` columns (`user_a` = requester, `user_b` = target by this package's convention) rather than renaming them - the table's uniqueness constraint is symmetric regardless of which column holds which side.
- `memory/` - in-memory `Repository` for tests.

## Storage

`relationships` table (`data/migrations/0001_core.sql`, `0008_relationships.sql`, which adds `updated_at`). At most one row exists per `(app_id, unordered user pair, relationship_type)`, enforced by the existing `LEAST`/`GREATEST` unique index.
