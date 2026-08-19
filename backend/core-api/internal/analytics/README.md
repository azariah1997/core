# Analytics

The roadmap's own envelope, implemented field for field: `event_name`, `user_id`, `anonymous_id`, `app_id`, `session_id`, `timestamp`, `properties`, `context`. "Operational databases must not become analytics databases" is the roadmap's own explicit constraint, and "create pipeline foundation for future ClickHouse/warehouse/data lake" is what this phase's real work actually is.

## The envelope is generic on purpose

`EventName` is free-form and product-defined (`"screen_viewed"`, `"purchase_completed"`, anything) - the same convention as every other Type/Name-shaped field in this repo. `Properties`/`Context` are arbitrary JSON. There is no fixed catalog of event names or a schema per event type - a generic envelope is the entire point.

## Ingestion is deliberately this platform's one open, unauthenticated write endpoint

Every other write in this platform goes through `requireUser` or `requireActive` - a verified Bearer token resolving to a real platform identity. `POST /v1/analytics/events` doesn't, on purpose: real analytics needs to capture pre-login and anonymous activity (an app open before signup, a marketing-site visit), which is exactly why the roadmap's own envelope names `anonymous_id` alongside `user_id` in the first place. Requiring a session would make half the envelope unusable. Real analytics SDKs (Segment, Amplitude, Mixpanel) all accept client-reported user/anonymous IDs without additional verification beyond a write key - this package's `UserID`/`AnonymousID` are exactly that trustworthy: self-declared, not cross-checked against a real session, a deliberate and accepted property of analytics tracking that's different from every other module's spoof-proof identity resolution.

Since there's no Bearer token to key anything by, `Track` rate-limits by the caller's IP address instead, reusing Phase 21's `platformkit/ratelimit.Limiter` for an unrelated purpose than it was built for (report-spam protection there; abuse protection for an endpoint with no auth boundary at all here) - confirmed live: 300 requests/minute allowed, the 301st `429`s, against real Valkey.

A single call accepts a **batch** of events (`{"events": [...]}`), matching how real client SDKs batch client-side before sending - a single event is just a batch of one.

## The landing table is not the analytics database

`analytics_events` plays the same role `outbox_events` (Phase 1) plays for domain events: a short-lived buffer, never queried for real analytical questions. `Service.ListRecent` is the one read path this package exposes, and it's deliberately narrow - platform.admin-only, capped at 200 rows, sorted by most-recent - a "did my last Track call actually land" debug tool, not a query surface. Its response literally says so (`"note": "operational debug view only..."`). Nothing in this platform ever runs an aggregation, funnel, or cohort query against this table.

## The pipeline: object storage as the landing zone

`backend/worker/internal/analyticspipeline` claims unflushed rows (`SELECT ... FOR UPDATE SKIP LOCKED`, the same claiming pattern Phase 15's job queue uses), batches them as newline-delimited JSON, and writes one object per batch to the platform's existing MinIO/S3 - a real, common "data lake" landing pattern (a ClickHouse `S3` table function, Redshift Spectrum, or a warehouse `COPY`/`LOAD` job would point at exactly this later), not a simulation of one. Confirmed live: tracked events showed `flushed: false` immediately after ingestion, `flushed: true` with a real `batchRef` after the next poll, and `mc cat` against the real MinIO container showed the object's actual NDJSON content - correctly using the roadmap's own field names (`event_name`, `anonymous_id`, `app_id`, `timestamp`, `properties`, `context`) verbatim, since that's exactly the shape a downstream loader would expect to read.

The write only marks rows flushed *after* the object storage write succeeds - a failed write leaves the claimed rows unflushed (the transaction rolls back) for the next poll to retry, rather than losing them.

### Why this runs in `worker`, and needed a new shared package

`worker` is a separate Go module from `core-api` and cannot import `core-api/internal/files/s3` - the same constraint documented since Phase 14's search indexer. Rather than duplicating that package's fuller (presigned-URL-oriented) surface, a new minimal shared package, `packages/go/platformkit/blobstore`, provides exactly what this pipeline needs: connect, ensure the bucket exists, `PutObject`. If a second consumer ever needs presigned URLs too, promoting `files/s3`'s implementation into this shared package (so `files` depends on it instead of the reverse) would be the natural next step - not done now, since it would touch Phase 13's already-validated code for a need that doesn't exist yet.

## Layout

- `domain.go` - `Event`, `TrackInput`, `Repository` (two methods only: `RecordBatch`, `ListRecent` - no analytical query methods).
- `service.go` - `Service`, `Track` (rate-limited, open), `ListRecent` (admin-only debug).
- `http.go` - `RegisterRoutes` (the debug listing, behind `requireUser`) and `RegisterTrackRoute` (the open ingestion endpoint, registered separately with no auth wrapper at all - see `router.go`).
- `postgres/`, `memory/` - `Repository` implementations.
- `backend/worker/internal/analyticspipeline` - the flush pipeline (a different Go module; can't live here).
- `packages/go/platformkit/blobstore` - the minimal shared S3 write client both this pipeline and any future consumer can use.

## Storage

`analytics_events` (`data/migrations/0021_analytics.sql`) - a new table, no pre-existing scaffold, with a partial index on unflushed rows so the pipeline's claiming query stays cheap even once millions of already-flushed rows accumulate.

## Not done here

No ClickHouse, warehouse, or data lake query layer exists yet - "pipeline foundation" is the literal scope, not the destination system itself, which this environment has no real instance of to integrate against honestly. The NDJSON batches landing in object storage are exactly what such a system would be pointed at next. No outbox event is emitted for tracked events either (this module *is* the pipeline entry point; adding a second layer of eventing on top of it would be redundant) - `contracts/asyncapi/events.yaml` is unchanged this phase, confirmed rather than silently skipped.
