# Privacy Compliance module

Owns the **privacy-compliance** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Two packages. `backend/core-api/internal/audit` is the platform's central, immutable audit trail - actor/action/resource/timestamp/correlation/app/tenant/device/metadata, immutable by both API omission and a database trigger; exposed read-only at `/v1/audit` (platform.admin-only). `backend/core-api/internal/privacy` owns append-only consent history, preferences, retention policy, and cross-module data export/deletion via an `Exporter`/`Deleter` registry (today's participants: users, devices, files; audit is export-only, deliberately never deletable) - coordinated by `core-api`'s own embedded Temporal worker; exposed at `/v1/privacy/*`. See each package's README for detail.
