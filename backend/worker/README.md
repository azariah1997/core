# Worker

Background processing for the platform: the search indexer (Phase 14) and the generic background job runner (Phase 15).

## `internal/indexer` - search indexing

The roadmap's own example made real: "`user.updated` → search index worker." `Indexer.PollOnce` claims up to 20 unpublished `outbox_events` rows whose `event_type` it recognizes (`user.created/updated/deactivated/deleted`, `application.created/updated`, `message.sent`), turns each into a `searchidx.Document` index or delete, applies it to OpenSearch, and marks the row published - all in one transaction (`SELECT ... FOR UPDATE SKIP LOCKED`, safe under concurrent worker replicas even though only one runs locally). `cmd/worker/main.go` runs this on a 2-second poll loop.

Unrecognized event types are left untouched (`published_at` stays `NULL`) rather than marked published - this indexer only claims responsibility for the events it actually understands, so a future outbox-to-Kafka relay (a documented gap since Phase 2 - this repo has no such worker yet) can still find and process every event, not just the ones already claimed here.

Indexed documents are thin: only the fields already present in the triggering event's payload, never a full re-read of the source entity. `worker` is a separate Go module from `core-api` and can't import its internal domain packages to fetch a complete record, and there's no service-to-service auth built yet to do it over HTTP instead - a deliberate, documented scope-down (see `backend/core-api/internal/search/README.md` for the full reasoning), not an oversight.

## `internal/jobrunner` - background job execution

The execution half of `core-api/internal/jobs`: that package only ever inserts a row (fast, no handler runs inside an HTTP request, per the roadmap's own "do not execute heavy jobs inside HTTP request handlers"); `Runner.PollOnce` here claims and actually runs due jobs, on a 1-second poll loop (`cmd/worker/main.go`).

Claiming (`SELECT ... FOR UPDATE SKIP LOCKED`, then marking `running`) happens in one short transaction; execution happens with no transaction open at all, deliberately unlike the search indexer above - a job's handler is explicitly allowed to be slow (a webhook call, for instance), and holding a Postgres row lock for the duration of that would itself be exactly the "heavy work" the roadmap says to keep off request-critical paths. On success, a recurring job (`RecurrenceIntervalSeconds` set) reschedules itself and resets its attempt counter; on failure, a job with retries remaining goes back to `scheduled` after an exponential backoff (`backoffFor`: 2s, 4s, 8s..., capped at 5 minutes); exhausting `MaxAttempts` sends it to `dead_letter`, a terminal state.

Two built-in handlers live in `internal/jobrunner/handlers`: `Echo` (logs and always succeeds) and `Webhook` (a genuine `POST` to a caller-supplied URL - real connection failures, timeouts, and non-2xx responses all count as real attempt failures, which is what makes this phase's retry/backoff/dead-letter behavior provable against a real failure in live validation, not a fabricated one).
