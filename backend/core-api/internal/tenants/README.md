# Tenants / Organizations

The platform's reusable multi-tenancy boundary. Consumer apps can ignore it entirely and operate as if every user has one implicit tenant; SaaS applications create many. Every tenant belongs to exactly one Application (Phase 2) - the platform never assumes all future applications are consumer-only single-tenant ones.

## Responsibilities

- Own `Tenant` (`id`, `appId`, `slug`, `name`, `status`, timestamps) and `Membership` (`tenantId`, `userId`, `role`).
- `Create` writes the tenant and an owner `Membership` for the creator in one transaction - a tenant can never transiently exist with no owner.
- Enforce membership-based access: any member can read; only `owner`/`admin` can update the tenant or manage other members' membership; any member can remove themselves ("leave") without elevated role.

## Non-responsibilities

- Tenant-role authorization ("is this user an owner/admin of *this* tenant") is answered directly from `Membership` data, not routed through `authz.Service`/OpenFGA. That's a deliberate choice, not an oversight: it's this package's own data, not a cross-domain relationship question - the kind `authz` exists for. If cross-tenant or hierarchical permission questions arise later, that's when routing through `authz` would earn its cost.
- "Organization" isn't a separate resource from `Tenant` - the roadmap names both without giving Organization distinct fields or endpoints, so this package treats them as one canonical resource (`Tenant`) that a product's own UI is free to label "Organization," "Workspace," "Team," or whatever fits.
- No endpoints were specified anywhere for this phase (unlike every phase before it) - `POST/GET /v1/tenants`, `GET/PATCH /v1/tenants/{id}`, and the `/members` sub-resource are this package's own reasonable design, not the roadmap's.

## Layout

- `domain.go` - `Tenant`, `Membership`, `Repository` interface, validation.
- `service.go` - `Service`, including the membership-based authorization (`requireMember`/`requireManager`).
- `http.go` - `POST/GET /v1/tenants`, `GET/PATCH /v1/tenants/{id}`, `GET/POST /v1/tenants/{id}/members`, `DELETE /v1/tenants/{id}/members/{userId}`. Takes `requireUser` as a parameter, same pattern as `devices`.
- `postgres/` - the production `Repository`.
- `memory/` - in-memory `Repository` for tests.

## Storage

`tenants` and `tenant_memberships` tables (`data/migrations/0007_tenants.sql`). `(app_id, slug)` and `(tenant_id, user_id)` are both unique.
