# Privacy

Phase 20's own list: consent, user data export, user deletion, retention policy, data lifecycle, privacy preferences. "Each module must be capable of participating in ExportUserData/DeleteUserData" and "do not create one giant cross-module delete query - coordinate deletion through workflows/events" are the roadmap's two explicit constraints, and shaped almost every design decision below.

## Responsibilities

- **Consent** (`Consent`) is append-only history, never updated in place - "did this user consent to X, and when" is a compliance question that needs the whole timeline, not just a current boolean. The current effective consent for a purpose is simply the newest row for it.
- **Privacy preferences** (`Preference`) are the opposite shape: current-value toggles (`"data_sharing": false`), the same structure as Phase 12's notification category opt-outs, just for a different concern.
- **Retention policy** (`RetentionPolicy`) is one current rule per `(AppID, ResourceType)` - a declared policy, not an enforcement engine; this package doesn't purge anything on a schedule, the same documented gap as Phase 13's `PurgeExpired` (callable, not scheduled).
- **Export and deletion** (`ExportRequest`/`DeletionRequest`) are the two big ones - see below.

## The Exporter/Deleter registry

"Each module must be capable of participating" is satisfied by two narrow interfaces:

```go
type Exporter interface { ExportUserData(ctx context.Context, userID string) (map[string]any, error) }
type Deleter  interface { DeleteUserData(ctx context.Context, userID string) error }
```

kept **separate**, not one combined interface, specifically so a participant that must never be erased (audit - see below) simply never implements `Deleter` at all. `Service.RegisterExporter`/`RegisterDeleter` are called once per participant from `cmd/server/main.go`, after every domain service already exists - adding a new participant later is one small adapter (`internal/api/privacy_adapters.go`) and two lines in `main.go`, never a change to this package.

### Four real participants, four different data shapes

| Module | Exports | Deletes |
|---|---|---|
| `users` | profile fields | `users.Service.Delete` (Phase 4's existing soft delete) |
| `devices` | device list, never raw push tokens | `devices.Service.RevokeAll` (new this phase) |
| `files` | file metadata only, never bytes | `files.Service.DeleteAllForUser` - real object storage deletes (new this phase) |
| `audit` | the user's own actions | **not registered as a Deleter at all** |

Audit is the one deliberately asymmetric participant: Phase 19 made audit records immutable by design (no `Update`/`Delete` method anywhere in its API, reinforced by a database trigger). A user is entitled to see what was recorded about their own actions (`audit.Service.ListMine`, new this phase - no admin check, unlike `List`/`Get`), but that history cannot be erased even by this phase's own deletion workflow. This isn't a special-cased `if` somewhere - the type system enforces it: `RunDeletion` only ever iterates registered `Deleter`s, and `auditPrivacyParticipant` was never given one.

### Deliberately deferred, with real reasons

- **relationships/groups/messaging**: shared, multi-user data. Deleting a relationship or a message outright would corrupt the *other* participant's own history - that needs a redaction/anonymization strategy this phase's "each participant acts on `userID` in isolation" assumption doesn't fit. A real follow-up, not solved here.
- **notifications**: the registry pattern is exactly what makes adding this later a small, isolated change rather than a framework change - proving that is more valuable this phase than wiring every module up front.
- **tenants/jobs/workflows/features/remoteconfig**: operational or administrative data about actions a user took, not personal data about the user - the same distinction GDPR itself draws.

## Export and deletion are coordinated through a workflow - one core-api runs itself

"Coordinate deletion through workflows/events, do not create one giant cross-module delete query" led to a genuinely new architectural choice for this repo: `internal/privacy/workflow` defines `ExportUserDataWorkflow`/`DeleteUserDataWorkflow` and runs its own embedded Temporal `worker.Worker`, started from `cmd/server/main.go` on its own task queue (`privacy.TaskQueue = "privacy-tasks"`) - **inside core-api itself**, not delegated to the separate `worker` service the way Phase 16's `ApprovalWorkflow`/`PingWebhookWorkflow` are.

That's deliberate, not an inconsistency: `worker` is a separate Go module and cannot import core-api's internal domain packages (the same constraint Phase 14 documented for why its search-index documents stay thin). Exporting or deleting a user's data means calling directly into core-api's own `users`/`devices`/`files`/`audit` services - only a worker living inside core-api's own process can do that. Temporal still earns its keep for the same reason Phase 16 chose it: durable, retryable, multi-step execution that survives a process restart mid-run. It's simply core-api running a second worker pool on a task queue of its own, the way a large system typically has more than one, each owned by whichever service can actually do the work.

`internal/privacy/workflow` is consequently the second (and only other) place in core-api that imports the Temporal SDK directly, alongside `internal/workflows/temporal` - that package only ever needs a client (`Start`/`Describe`/`Signal`); this one also needs to run a `worker.Worker`, since it has to actually execute code, not just ask Temporal to.

### Postgres, not Temporal, is the caller-facing source of truth

Unlike Phase 16's `WorkflowRun` (which deliberately never duplicates Temporal's execution state), `ExportRequest`/`DeletionRequest` **do** persist their own status, updated directly by the activity as it runs. The caller-facing contract here is "is my request done," not "let me inspect a workflow" - callers never need to know Temporal is involved at all. `GET /v1/privacy/export/{id}` and `.../delete/{id}` read straight from Postgres; there is no `Describe`-style call to Temporal anywhere in this package. Each workflow itself is consequently almost trivial - one retryable activity call - because the real coordination logic (fan out to every registered participant, tolerate individual failures, record the outcome) lives in `Service.RunExport`/`RunDeletion`, plain Go testable with fakes and no Temporal test harness required.

A request's returned status always comes from a **re-fetch** after starting the workflow, never a hand-patched in-memory value - live validation caught this exact bug: the embedded worker can run and complete an activity before the HTTP handler that started it even returns, since it's in the same process. A hand-patched "pending"/"running" would understate real progress exactly as often as it overstates it.

### Tolerant, not all-or-nothing

`RunExport` and `RunDeletion` both attempt every registered participant regardless of an earlier one failing. One module's export error lands in the bundle as an `"error"` entry for that module, not a failed export - a user's data from every *other* module should still be retrievable even if one participant is down. Deletion is the same: a user blocked by, say, a storage outage on `files` still gets their `users`/`devices` data erased, with the partial outcome recorded in `Results` for an admin to see and retry.

## A confirmed, real interaction worth knowing about

Live validation surfaced this directly, not a bug: once `RunDeletion`'s `users` participant calls `users.Service.Delete`, the user's own login token can no longer resolve to *any* authenticated endpoint on the platform - including checking the very deletion request they just started. This is pre-existing Phase 4 behavior (`users.ErrDeleted` is "treated like `ErrNotFound` by callers"), simply exercised live for the first time in this session, since no earlier phase's validation ever called `Delete` through the full `requireUser` → `EnsureForIdentity` → `Get` chain with a real subsequent request. It's the correct, defensible posture - a deleted account should lose all platform access immediately, the same way deleting a Google account logs you out everywhere - so `GET /v1/privacy/delete/{id}` for a user's *own* completed self-deletion must be checked by a platform.admin instead. A real product would pair this with a side channel (a confirmation email, sent before the account can no longer receive one) - out of this phase's scope.

## Export bundles are uploaded directly, not presigned

Every other write in `files` (Phase 13) hands the client a presigned PUT URL - this service never receives file bytes. Privacy's export bundle is different: it's assembled by this service itself, in-process, with no client to hand a URL to. `files/s3.Store` gained a `PutObject` method this phase (its first server-side write) specifically for this; `privacy.ExportStore` is satisfied directly by the same `*filess3.Store` already constructed for `files.Service`, no adapter needed. Downloading the finished bundle still goes through a presigned GET, same as every other file download on this platform.

## Layout

- `domain.go` - `Consent`, `Preference`, `RetentionPolicy`, `ExportRequest`, `DeletionRequest`, `Exporter`/`Deleter`, `Repository`.
- `service.go` - `Service`, the registry, `RunExport`/`RunDeletion` (called by the workflow activities), admin/self gating.
- `http.go` - the REST surface.
- `postgres/`, `memory/` - `Repository` implementations.
- `workflow/` - the Temporal workflow/activity definitions and the embedded worker (see above).

## Storage

`privacy_consents`, `privacy_preferences`, `retention_policies`, `data_export_requests`, `data_deletion_requests` (`data/migrations/0018_privacy.sql`) - all new tables, no pre-existing scaffold.

## Not done here

No outbox event is emitted anywhere in this package - matching Phase 19's audit (direct/synchronous by design) rather than Phase 14/18's event-driven pattern, since export/deletion requests already have their own durable, pollable status via Postgres; `contracts/asyncapi/events.yaml` is unchanged this phase for the same reason, confirmed rather than silently skipped.
