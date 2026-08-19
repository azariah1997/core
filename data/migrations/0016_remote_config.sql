BEGIN;

-- Fully new tables, no pre-existing scaffold, same as jobs/workflow_runs/
-- features. Deliberately separate from features/feature_rules (0015) -
-- "separate configuration from feature flags" is the roadmap's own
-- instruction: this is a typed key/value store for arbitrary settings
-- (limits, URLs, UI options, minimum versions, maintenance state), not a
-- targeting-rule evaluation engine.
CREATE TABLE IF NOT EXISTS config_entries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES applications(id),
  -- Free-form, e.g. "production", "staging" - no built-in fallback/
  -- inheritance between environments; a caller wanting a value shared
  -- across environments just uses the same environment string
  -- (e.g. "*") for all of them by convention, not a platform feature.
  environment text NOT NULL,
  key text NOT NULL, -- product-defined, e.g. "checkout.maxRetries"
  value jsonb NOT NULL, -- arbitrary JSON - a string, number, bool, object, or array
  description text NOT NULL DEFAULT '',
  updated_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_config_entries_scope ON config_entries(app_id, environment, key);

-- "All changes must be auditable" - the roadmap's own explicit
-- requirement for this phase. One row per write (including deletes,
-- recorded as new_value = NULL), kept indefinitely as full history, never
-- updated or deleted itself.
CREATE TABLE IF NOT EXISTS config_changes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES applications(id),
  environment text NOT NULL,
  key text NOT NULL,
  previous_value jsonb,
  new_value jsonb,
  changed_by uuid REFERENCES users(id),
  reason text NOT NULL DEFAULT '',
  changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_config_changes_scope ON config_changes(app_id, environment, key, changed_at DESC);

COMMIT;
