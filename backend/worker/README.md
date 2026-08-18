# Worker

Background processing for the platform. Currently owns one job: the search indexer.

## `internal/indexer` - search indexing

The roadmap's own example made real: "`user.updated` → search index worker." `Indexer.PollOnce` claims up to 20 unpublished `outbox_events` rows whose `event_type` it recognizes (`user.created/updated/deactivated/deleted`, `application.created/updated`, `message.sent`), turns each into a `searchidx.Document` index or delete, applies it to OpenSearch, and marks the row published - all in one transaction (`SELECT ... FOR UPDATE SKIP LOCKED`, safe under concurrent worker replicas even though only one runs locally). `cmd/worker/main.go` runs this on a 2-second poll loop.

Unrecognized event types are left untouched (`published_at` stays `NULL`) rather than marked published - this indexer only claims responsibility for the events it actually understands, so a future outbox-to-Kafka relay (a documented gap since Phase 2 - this repo has no such worker yet) can still find and process every event, not just the ones already claimed here.

Indexed documents are thin: only the fields already present in the triggering event's payload, never a full re-read of the source entity. `worker` is a separate Go module from `core-api` and can't import its internal domain packages to fetch a complete record, and there's no service-to-service auth built yet to do it over HTTP instead - a deliberate, documented scope-down (see `backend/core-api/internal/search/README.md` for the full reasoning), not an oversight.
