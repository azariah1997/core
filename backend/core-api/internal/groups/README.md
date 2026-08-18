# Groups / Circles

A generic grouping primitive. Friend circles, families, teams, communities, workspaces, gaming guilds are all the same shape to the platform - nothing here hardcodes any one of those use cases.

## Responsibilities

- Own `Group` (name, status, product-defined `metadata`) and `GroupMember` (`role`, `isManager`).
- `Role` is a free-form, product-defined label ("parent", "leader", "officer", "recruit" - whatever the product wants) and is **never** validated against a fixed enum or used for authorization decisions.
- `IsManager` is the one structural permission bit the platform itself enforces, deliberately orthogonal to `Role`'s label: a "parent" and a "leader" might both be managers in their respective products, or might not be - the platform doesn't know or care what the label means, only whether the bit is set.
- `Create` writes the group and a manager membership for the creator in one transaction - a group can never transiently exist with no manager.
- Any manager can add/remove members, update the group, and promote/demote other members' `IsManager` bit (including, deliberately, their own or the original creator's - this package doesn't invent an "owner outranks admin" hierarchy the roadmap never asked for; if a product wants that, it's a policy the product enforces on top of this primitive).

## Non-responsibilities

- Doesn't decide what a group's `Role` labels should be, or enforce any hierarchy among them - that's entirely the product's business logic, carried in `Role` and `metadata`.
- No endpoints were specified anywhere for this phase (same as Phases 7/8) - the endpoint set is this package's own design, closely mirroring `tenants`' shape since group membership management is structurally the same problem.
- No events emitted - the roadmap doesn't name any for this phase, matching Phase 7's precedent (as opposed to Phase 8, where `relationship.created` was explicitly named).

## Layout

- `domain.go` - `Group`, `GroupMember`, `Repository` interface, validation.
- `service.go` - `Service`, including the manager-based authorization.
- `http.go` - `POST/GET /v1/groups`, `GET/PATCH /v1/groups/{id}`, `GET/POST /v1/groups/{id}/members`, `PATCH/DELETE /v1/groups/{id}/members/{userId}`.
- `postgres/` - the production `Repository`.
- `memory/` - in-memory `Repository` for tests.

## Storage

`groups` and `group_members` tables (`data/migrations/0009_groups.sql`). `(group_id, user_id)` is unique.
