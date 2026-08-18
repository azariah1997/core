# Workflows

The Core Workflow API/SDK the roadmap asks for - "do not expose Temporal directly to product applications" is its whole reason to exist. `Service.Start`/`Get`/`Signal` are what callers use; Temporal itself, and the actual workflow/activity code, are never reachable from outside this package (and outside `backend/worker/internal/workflows`, which is where that code actually runs).

## Responsibilities

- `Start` begins a named (free-form `Type` string, like every other Type field in this repo) workflow execution on the shared task queue (`platformkit/workflowkit.TaskQueue`) and records an ownership row.
- `Get` returns the *live* execution state - status, and result or error once terminal - queried straight from Temporal via `DescribeWorkflowExecution` (and `GetWorkflow(...).Get()` for the actual result once completed). Nothing about execution state is cached or duplicated in Postgres; Temporal is the source of truth for it, the same "don't duplicate the real source of truth" principle `files` applies to S3/MinIO and `search` applies to OpenSearch.
- `Signal` sends a named signal with an optional payload to a running workflow - how a long-running workflow (`ApprovalWorkflow` waiting on a human decision, for instance) receives external input.
- `Start`'s `CronSchedule` field passes straight through to Temporal's own native recurring-execution feature - "scheduled processes" needs nothing hand-rolled here, unlike Phase 15's job queue, which had to build its own poll loop because it isn't backed by a workflow engine.

## Scoping decisions

- **Self-or-platform.admin**, same as every other module's ownership pattern. A `workflow_runs` Postgres table (`workflow_id`, `run_id`, `type`, `created_by`) exists purely for this authorization check - it is not execution state, and is never consulted for anything else.
- **The workflow ID is always server-generated** (`type-uuid`), never caller-supplied, so ownership can never be spoofed by guessing or reusing someone else's workflow ID.
- **No cancel/terminate endpoint.** Not named in the roadmap's capability list (long-running operations, multi-step processes, retries, compensation, scheduled processes); an operator can still terminate a workflow directly via Temporal's own tooling (`tctl`/Temporal Web UI) without this package needing to expose it.

## Layout

- `domain.go` - `WorkflowRun` (ownership bookkeeping), `Execution` (a live Temporal snapshot, never persisted), `Status`, `StartInput`/`SignalInput`, `Repository` interface.
- `service.go` - `Service` and the narrow `TemporalClient` interface it depends on (`Start`/`Describe`/`Signal` only, not the full Temporal SDK client) - the actual embodiment of "do not expose Temporal directly": nothing outside `internal/workflows/temporal` ever imports `go.temporal.io/sdk/client`.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `temporal/` - the real `TemporalClient` implementation, the only place in `core-api` that imports the Temporal SDK.
- `postgres/`, `memory/` - `Repository` implementations for the ownership table.

## The built-in workflows: `backend/worker/internal/workflows`

Two workflow types ship, both demonstrating every capability the roadmap names, using a real HTTP call (reusing Phase 15's honest `Webhook` job handler logic, not a fabricated activity) so live validation could prove the mechanics against real behavior, not simulated:

- **`approval`** - multi-step (prepare → wait → approve/reject) + long-running (waits indefinitely for a signal via a durable timer that survives worker restarts) + retries (each activity runs under a Temporal-native `RetryPolicy` - automatic, not hand-rolled) + compensation (a `reject` signal and a timeout both run the same "undo" activity a plain approval would skip).
- **`ping_webhook`** - a trivial single-activity workflow whose only purpose is to be startable with a `CronSchedule`, demonstrating scheduled processes as Temporal's own native feature.

One real bug caught only by live validation, not any unit test: the worker initially registered these under their Go function names (`worker.RegisterWorkflow` defaults to that), while `Service.Start` addresses workflows by the free-form string `Type` ("approval", "ping_webhook") - a mismatch that let the `Start` call itself succeed while the workflow sat retrying "unable to find workflow type" forever, invisible from the caller's side. Fixed with `RegisterWorkflowWithOptions(fn, workflow.RegisterOptions{Name: "approval"})`, registering under the exact string `Service.Start` uses.
