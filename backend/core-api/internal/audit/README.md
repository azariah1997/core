# Audit

The roadmap's own instruction: "Implement central Audit Service. Audit records should include: actor, action, resource, timestamp, correlation ID, application, tenant, device context when appropriate, metadata. Audit must be immutable from normal application APIs."

## Responsibilities

- `Record` is one immutable fact: who (`ActorUserID`) did what (`Action`, a free-form string like `role.assigned`) to what (`ResourceType`/`ResourceID`), when (`OccurredAt`, server-set), correlated to the originating request (`CorrelationID`, pulled from `platformkit/correlation`, also server-set - never client-supplied), scoped to an application/tenant/device when relevant (`AppID`/`TenantID`/`DeviceID`, all optional), with arbitrary structured context (`Metadata`).
- `Record` (the write) is open to **any authenticated caller** - any module in the platform, acting on behalf of the user making the request, can log an audit event about its own action. The handler always overrides `ActorUserID` with the authenticated caller's ID before it reaches the service, so a caller can never submit an audit record claiming to be someone else.
- `Get`/`List` (reading the trail) are **platform.admin only** - an audit log is only useful as a security/compliance tool if ordinary users can't browse each other's history.

## "Immutable from normal application APIs" - enforced twice

1. **By omission.** `Repository` (`domain.go`) has exactly three methods: `Record`, `Get`, `List`. There is no `Update` or `Delete` anywhere in the Go API - not on `Repository`, not on `Service`, not as an HTTP route. The strongest way to guarantee an API can't be misused is for it not to exist.
2. **At the database.** `data/migrations/0017_audit.sql` adds a `BEFORE UPDATE OR DELETE` trigger on `audit_events` that unconditionally raises an exception. Confirmed live: a raw `UPDATE`/`DELETE` issued directly over `psql` - bypassing this repo's Go code entirely - is rejected. This is defense-in-depth beyond every other phase's single-layer (API-only) enforcement, because an audit trail that a bug, a careless migration, or a direct database session could quietly rewrite isn't actually a trustworthy audit trail.

## Why direct/synchronous, not outbox-based

Phase 14 (search) and Phase 18 (remote config) both write via the transactional outbox (`outbox_events`), consumed asynchronously by a worker. Audit deliberately does not: a security-relevant record that "will show up eventually, once a worker gets around to it" is a weaker guarantee than this data needs. `Service.Record` writes directly and synchronously to `audit_events` in the same request that triggered it - the caller gets a definitive success/failure, not an eventual-consistency promise.

## Why only one real integration (`authz`) for now

`authz.Service.AssignRole`/`RevokeRole` - granting or revoking a role, including `platform.admin` - is the one action in the platform that was already a natural, unambiguous fit for "this needs an audit trail" without inventing a use case. Rather than retrofitting every module's every write into an audit call (a large, speculative change this phase's scope didn't ask for), only this one genuinely compelling integration was wired, matching the precedent Phases 15/16 set of one or two real demonstration integrations rather than exhaustive coverage. Adding more producers later means implementing `authz.AuditRecorder`'s pattern (a narrow, primitive-typed interface + a small adapter) against `audit.Service`, or simply calling `POST /v1/audit` directly - both are already proven live.

## The authz <-> audit construction cycle

`authz.Service` needs to report role changes to `audit.Service`. `audit.Service` needs `authz.Service` as its `AdminChecker` (to gate `Get`/`List`). Neither can be constructed first. Broken with a two-phase wiring adapter, `api.RoleChangeAuditRecorder` (`internal/api/audit_adapter.go`): built empty and passed into `authz.NewService`, then `audit.Service` is built using the already-real `authz.Service` as its `AdminChecker`, then `SetAuditService` completes the recorder's wiring. Safe because `RecordRoleChange` - the only method that dereferences the field `SetAuditService` sets - is never called during synchronous startup, only later from an actual `AssignRole`/`RevokeRole` API call, by which point wiring has always already completed. Confirmed live via a throwaway program: assigning then revoking a role produced exactly the two expected `role.assigned`/`role.revoked` audit records, each with the correct actor, target, and role in `Metadata`.

The audit write on the authz side is best-effort: a failure to record is logged, not returned, so an audit-service hiccup can never block a legitimate role change - the same "a best-effort side effect can't fail the primary operation" principle `messaging`/`notifications` already apply to realtime pushes.

## Layout

- `domain.go` - `Record`, `RecordInput`, `ListFilter`/`ListResult`, `Repository` interface (no Update/Delete).
- `service.go` - `Service`, `AdminChecker` interface, admin gating for `Get`/`List`, open `Record`.
- `http.go` - three routes only: `POST /v1/audit`, `GET /v1/audit`, `GET /v1/audit/{id}`.
- `postgres/` - the production `Repository`.
- `memory/` - in-memory `Repository` for tests, also with no Update/Delete.

## Storage

`audit_events` (from the Phase 1 scaffold, `data/migrations/0001_core.sql`, confirmed empty before this phase), extended by `data/migrations/0017_audit.sql` with `tenant_id`/`device_id` and the immutability trigger.

## Not done here

No outbox event is emitted for audit writes - see "why direct/synchronous" above; this is a deliberate scoping decision, not an oversight, and `contracts/asyncapi/events.yaml` is unchanged this phase for the same reason (matching Phase 15/16's precedent of explicitly noting when a phase has no event to add, rather than silently skipping it).
