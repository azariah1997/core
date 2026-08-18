BEGIN;

-- Ownership/audit bookkeeping only - execution state (status, result,
-- history) lives in Temporal, queried live, never duplicated here. No
-- pre-existing scaffold table for this one, same as jobs (0013).
CREATE TABLE IF NOT EXISTS workflow_runs (
  workflow_id text PRIMARY KEY,
  run_id text NOT NULL,
  -- Product/platform-defined, free-form (e.g. "approval", "ping_webhook") -
  -- same convention as every other Type field in this repo.
  type text NOT NULL,
  created_by uuid REFERENCES users(id), -- nullable: system-started workflows have no caller
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_workflow_runs_created_by ON workflow_runs(created_by, created_at DESC);

COMMIT;
