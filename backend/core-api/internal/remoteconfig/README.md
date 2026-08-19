# Remote Configuration

"Separate configuration from feature flags" is the roadmap's own instruction distinguishing this from Phase 17. This is a typed key/value store for dynamic settings - limits, URLs, UI options, service behaviour, minimum versions, maintenance state are the roadmap's own examples of what a value might hold, not a fixed set of supported keys or a targeting-rule evaluation engine.

## Responsibilities

- `Entry` is the current, live value for one `(AppID, Environment, Key)`. `Value` is arbitrary JSON - a string, number, bool, object, or array - since config values are too varied in shape for a fixed schema (a URL is a string, a limit is a number, "UI options" or "maintenance state" could be an object).
- **"All changes must be auditable"** is the roadmap's own explicit requirement for this phase, and the reason `Change` exists: every `Set` or `Delete` writes an Entry update *and* a `Change` row (previous value, new value, who, when, an optional free-text reason) in the same transaction. A delete's `Change` row survives even after the `Entry` itself is gone - confirmed live: a deleted key's full history, including its final previous-value, remains queryable after the key returns 404.
- Every write also emits a `config.updated`/`config.deleted` outbox event - a seam a future central Audit Service (Phase 19) could consume via the same outbox-polling pattern Phase 14's search indexer already established, without this package needing to know that service exists yet.

## Scoping decisions

- **No fallback or inheritance between environments.** `(AppID, Environment, Key)` is an exact-match scope; setting `production` and `staging` values for the same key are two entirely independent entries, confirmed live. A product wanting a "global default with per-environment overrides" would layer that convention on top (e.g. a shared `Environment` string like `"*"`), not get it from this package automatically - environment fallback resolution is real complexity this phase's scope didn't call for.
- **Management (`Set`/`Delete`) and `History` are platform.admin only; `Get`/`List` are open to any authenticated caller** - a running service or client needs to read its own config at runtime, and config values aren't sensitive per-user data, but an audit trail of who-changed-what is a more privileged, operationally-scoped concern (the same split `features` uses for management vs. evaluation).
- **Maintenance mode is just a config entry**, not bespoke machinery - e.g. `key: "maintenance"`, `value: {"enabled": true, "message": "..."}`. Keeping it a plain config value (rather than a dedicated maintenance-mode construct) matches this repo's "generic platform capability, not hardcoded product concepts" convention running through every phase.

## Layout

- `domain.go` - `Entry`, `Change`, `SetInput`, `Repository` interface.
- `service.go` - `Service`, admin gating for writes and history, open reads.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `postgres/` - the production `Repository`. `Set` and `Delete` are each one transaction covering the entry upsert/delete, the `Change` audit row, and the outbox event - the same transactional discipline every other write-path `Repository` in this repo already uses, even though this module doesn't otherwise need the transactional-outbox pattern for its own purposes.
- `memory/` - in-memory `Repository` for tests.

## Storage

`config_entries`, `config_changes` (`data/migrations/0016_remote_config.sql`) - fully new tables, no pre-existing scaffold (same as `jobs`, `workflow_runs`, `features`).
