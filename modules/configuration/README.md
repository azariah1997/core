# Configuration module

Owns the **configuration** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Two packages. `backend/core-api/internal/features` owns `Feature`/`FeatureRule`/`FeatureEvaluation`, targeting environment/user/tenant/platform/country/percentage/version, with deterministic FNV-1a percentage-rollout bucketing so a given user consistently lands on the same side of a rollout; exposed at `/v1/features*`. `backend/core-api/internal/remoteconfig` owns a typed key/value store scoped by `(AppID, Environment, Key)` with a full change-audit trail and `config.updated`/`config.deleted` domain events; exposed at `/v1/config*`. See each package's README for detail.
