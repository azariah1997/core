BEGIN;

-- Consent is append-only history, never updated in place: "did this user
-- consent to X, and when" is a compliance question that needs the full
-- timeline, not just a current boolean - the same reasoning audit_events
-- (Phase 19) applies to actions in general, narrowed here to consent
-- specifically. The current effective consent for a purpose is simply
-- the most recent row for that (user_id, purpose) pair.
CREATE TABLE IF NOT EXISTS privacy_consents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  purpose text NOT NULL,
  granted boolean NOT NULL,
  version text NOT NULL DEFAULT '1',
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_privacy_consents_user ON privacy_consents(user_id, purpose, recorded_at DESC);

-- Privacy preferences, unlike consent, are current-value toggles (e.g.
-- "data_sharing": false) - no history needed, the same shape as
-- Phase 12's notification category opt-outs, just for a different
-- concern.
CREATE TABLE IF NOT EXISTS privacy_preferences (
  user_id uuid NOT NULL REFERENCES users(id),
  key text NOT NULL,
  value boolean NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, key)
);

-- One current rule per (app, resource type) - retention policy is an
-- operational setting, not something that needs its own change history
-- the way config_changes (Phase 18) tracks remote config.
CREATE TABLE IF NOT EXISTS retention_policies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid REFERENCES applications(id),
  resource_type text NOT NULL,
  retention_days integer NOT NULL,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (app_id, resource_type)
);

-- Export/deletion requests are Postgres-status-is-truth (like Phase 15's
-- Job), not Temporal-is-truth (like Phase 16's WorkflowRun) - see
-- privacy/README.md for why: callers never need to know Temporal is
-- involved at all, only whether their request is done.
CREATE TABLE IF NOT EXISTS data_export_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  status text NOT NULL DEFAULT 'pending',
  workflow_id text,
  run_id text,
  object_key text,
  error text,
  requested_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
CREATE INDEX IF NOT EXISTS ix_data_export_requests_user ON data_export_requests(user_id, requested_at DESC);

CREATE TABLE IF NOT EXISTS data_deletion_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  status text NOT NULL DEFAULT 'pending',
  workflow_id text,
  run_id text,
  results jsonb NOT NULL DEFAULT '{}'::jsonb,
  error text,
  requested_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
CREATE INDEX IF NOT EXISTS ix_data_deletion_requests_user ON data_deletion_requests(user_id, requested_at DESC);

COMMIT;
