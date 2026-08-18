# File / Media Platform

Presigned upload/download against S3-compatible object storage (MinIO locally, real S3 in production - one adapter, not two), with metadata/ownership/permissions/size limits/MIME validation/checksum/delete/retention in Postgres. Binary bytes are never stored in Postgres and never proxied through this service - the client uploads and downloads directly against a presigned URL, and this service never receives the bytes at all.

## Responsibilities

- `RequestUpload` validates size/MIME type, creates a `Pending` `File` row, and returns a presigned PUT URL.
- `ConfirmUpload` reads the object's *real* size and checksum from storage (a `HEAD` request) and writes those - never the client's declared values - marking the file `Active`. If the client declared a checksum up front, a mismatch against what's actually in storage is rejected.
- `GetDownloadURL` returns a presigned GET URL for an `Active` file the caller may see (owner, `public` visibility, or platform.admin).
- `Delete` removes the object from storage first, then soft-deletes the row - never the other way around, so a storage failure can't leave a row claiming a file is gone when its bytes still exist.
- `PurgeExpired` deletes every `Active` file past its retention `ExpiresAt`.
- Emits `file.uploaded` via the transactional outbox on confirm - the roadmap's own example event, and the hook point for the thumbnail generation / image optimization / virus scanning / media processing the roadmap asks to "prepare hooks for." None of those are implemented here; a future worker subscribing to `file.uploaded` is where they'd live.

## Scoping decisions

- **No real MIME-sniffing or virus scanning.** `MimeType` is the client's declared content type, validated only against a configurable allowed-prefix list (e.g. `"image/"`) - not sniffed from the actual bytes, which this service never sees. Actually inspecting file content is exactly the "virus scanning / media processing" hook this phase prepares for but doesn't implement.
- **Checksum is MD5 via S3's ETag**, not a stronger hash. For a simple (non-multipart) `PUT`, an S3-compatible store's ETag is the object's MD5 hex - good enough to catch corruption/tampering between what a client declared and what actually landed in storage, which is the property this phase's live validation confirmed (a declared-wrong checksum against real uploaded bytes was correctly rejected). A product wanting SHA-256 integrity guarantees would need its own out-of-band verification.
- **Retention has no background scheduler.** `PurgeExpired` is a callable Service method and an admin-only REST endpoint, not something a cron fires automatically - the same documented gap as this platform's outbox-to-Kafka relay and Phase 12's notification retries.
- **Send/manage-others requires no special role beyond ownership** - `Get`/`GetDownloadURL`/`Delete` are self-or-platform.admin (reusing Phase 6's `authz.Service`, satisfied directly by the identical `IsPlatformAdmin` method signature, no adapter needed), and `public` visibility bypasses the ownership check entirely. `PurgeExpired` is platform.admin-only, since it acts across all owners' files.

## Layout

- `domain.go` - types, validation, `Repository` interface.
- `service.go` - `Service`, the `ObjectStore`/`AdminChecker` interfaces it depends on, object-key construction, and the upload/confirm/download/delete/purge logic.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `s3/` - the production `ObjectStore`, using `aws-sdk-go-v2`'s S3 client against any S3-compatible endpoint (MinIO locally). Ensures the configured bucket exists at startup, self-healing the same way OpenFGA's store/model and Keycloak's realm import do, since MinIO's local dev storage is exactly as ephemeral as those.
- `postgres/` - the production `Repository`, built on the pre-existing scaffold `files` table (extended by `0012_files.sql` with `file_name`/`status`/`checksum`/`expires_at`/`updated_at`).
- `memory/` - in-memory `Repository` for tests.

## Storage

`files` (`data/migrations/0001_core.sql`, extended by `0012_files.sql`). Object keys are namespaced `appId/ownerUserId/uuid-sanitizedFileName`, so two users (or two apps sharing the platform) can never collide, and re-uploading a file with the same name never overwrites a still-referenced object.
