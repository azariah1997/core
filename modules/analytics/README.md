# Analytics module

Owns the **analytics** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/analytics` - a generic event envelope (`eventName`/`userId`/`anonymousId`/`appId`/`sessionId`/`timestamp`/`properties`/`context`) ingested at `POST /v1/analytics/events`, this platform's one deliberately open, unauthenticated write endpoint (IP-rate-limited instead of Bearer-gated, since real analytics needs pre-login/anonymous activity). `analytics_events` is a short-lived landing buffer, never an analytics query surface itself - "operational databases must not become analytics databases" is enforced by a `worker` pipeline (`internal/analyticspipeline`) that batches unflushed rows as NDJSON into MinIO/S3, the real landing pattern a ClickHouse/warehouse destination would read from next. See that package's README for detail.
