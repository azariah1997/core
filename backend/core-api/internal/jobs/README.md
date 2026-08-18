# Background Jobs

The platform's generic background job queue. `Service.Enqueue` is the entire cost of scheduling work from an HTTP request - a single insert - never a handler running inline. "Do not execute heavy jobs inside HTTP request handlers" is the roadmap's own instruction; execution happens later, in the worker process (`backend/worker/internal/jobrunner`).

## Responsibilities

- `Job`/`JobStatus`/`JobAttempt` - the three constructs the roadmap names. A `Job`'s `Status` has no "failed" state: a failed attempt with retries remaining goes back to `scheduled` with a later `RunAt`; only exhausting every attempt reaches the terminal `dead_letter`. Individual attempt outcomes live on `JobAttempt`, one row per execution, kept even after the job reaches a terminal state as the retry/dead-letter audit trail.
- All six roadmap capabilities: **immediate** (`RunAt` defaults to now), **scheduled** (`RunAt` set to a specific future time), **delayed** (`DelaySeconds`, sugar for "now + N seconds," mutually exclusive with `RunAt`), **recurring** (`RecurrenceIntervalSeconds` - on success the same row reschedules itself and resets its attempt counter for a fresh cycle), **retry** (exponential backoff, capped), and **dead-letter** (a terminal state once `MaxAttempts` is exhausted).

## Scoping decisions

- **No outbox event on enqueue**, unlike every other write-path `Repository` in this repo. A job's own execution (recorded via `JobAttempt`) already produces a clear audit trail, and nothing external needs to react to "a job was enqueued" the way other domains' create events matter.
- **Any authenticated caller may enqueue**; `Get`/`ListAttempts` are self-or-platform.admin, `ListMine` is always scoped to the caller. Consistent with this repo's established self-or-admin pattern (`AdminChecker` satisfied directly by `*authz.Service`, no adapter needed).
- **No cancel/pause endpoint.** The roadmap's capability list is immediate/scheduled/delayed/recurring/retry/dead-letter - cancellation isn't named, so it isn't built.

## Layout

- `domain.go` - `Job`, `JobStatus`, `JobAttempt`, `EnqueueInput` (with the `RunAt`/`DelaySeconds`/`RecurrenceIntervalSeconds` resolution logic), `Repository` interface.
- `service.go` - `Service`: `Enqueue`, `Get`, `ListMine`, `ListAttempts`, and the `AdminChecker` dependency.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `postgres/` - the production `Repository`. Shares the `jobs`/`job_attempts` tables with worker's `internal/jobrunner`, which claims and executes the rows this package only ever inserts and reads - `core-api` and `worker` are separate Go modules that can't import each other's internal packages, so the database table is the entire contract between "enqueue" and "execute," the same split as `search`'s outbox-polling indexer.
- `memory/` - in-memory `Repository` for tests.

## Storage

`jobs`, `job_attempts` (`data/migrations/0013_jobs.sql`) - fully new tables, unlike `relationships`/`messaging`/`notifications`/`files`, none of which had a pre-existing scaffold table to adapt.

## Execution: `backend/worker/internal/jobrunner`

Claiming (`SELECT ... FOR UPDATE SKIP LOCKED`, then marking `running`) happens in one short transaction; execution happens with **no** transaction open at all - a job's handler is explicitly allowed to be slow (a webhook call, for instance), and holding a Postgres row lock for the duration of that would itself be exactly the kind of request-critical-path heavy work the roadmap says to avoid. This is a deliberate difference from Phase 14's search indexer, which does hold its claiming transaction open through a (fast) OpenSearch call - jobs make no such speed assumption about their handlers.

Two built-in handlers ship in `handlers/`: `Echo` (logs and always succeeds - an honest smoke-test primitive) and `Webhook` (a genuine `POST` to a caller-supplied URL - can really fail on a bad URL, a connection refused, a timeout, or a non-2xx response, which is what makes this phase's live validation of retry/backoff/dead-letter provable against real failures rather than fabricated ones). Product-specific job types would register their own handlers the same way.
