# Content module

Owns the **content** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Two packages. `backend/core-api/internal/files` owns presigned upload/download against real MinIO/S3, ownership, visibility, retention, and real MD5 checksum verification - exposed at `/v1/files*`. `backend/core-api/internal/search` owns `SearchDocument`/`SearchProvider` over a real OpenSearch cluster, kept in sync by `worker`'s event-driven indexer polling the outbox rather than search writes happening inline in HTTP handlers - exposed at `/v1/search*`. See each package's README for detail.
