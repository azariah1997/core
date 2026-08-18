BEGIN;

-- No pre-existing scaffold table for this one (unlike relationships,
-- messaging, notifications, files) - a fully new capability.
CREATE TABLE IF NOT EXISTS jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid REFERENCES applications(id), -- nullable: platform-global jobs are allowed
  -- Product/platform-defined, free-form (e.g. "webhook", "echo") - same
  -- convention as every other Type field in this repo, never a fixed enum.
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- scheduled: waiting for run_at. running: claimed by a worker.
  -- succeeded: terminal. dead_letter: terminal, retries exhausted.
  status text NOT NULL DEFAULT 'scheduled',
  run_at timestamptz NOT NULL DEFAULT now(), -- immediate = now(); scheduled/delayed = a future time
  -- NULL = one-shot job. Set = recurring: on success the same row is
  -- rescheduled run_at = now() + this interval, attempts reset to 0.
  recurrence_interval_seconds integer,
  max_attempts integer NOT NULL DEFAULT 5,
  attempts integer NOT NULL DEFAULT 0,
  locked_at timestamptz,
  created_by uuid REFERENCES users(id), -- nullable: system-enqueued jobs have no caller
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
-- The worker's poll query is exactly "due, waiting" jobs - a partial
-- index keeps that scan cheap regardless of how many succeeded/dead
-- jobs accumulate.
CREATE INDEX IF NOT EXISTS ix_jobs_due ON jobs(run_at) WHERE status = 'scheduled';
CREATE INDEX IF NOT EXISTS ix_jobs_created_by ON jobs(created_by, created_at DESC);

-- One row per execution attempt, kept even after the job reaches a
-- terminal state - the audit trail retry/dead-letter needs.
CREATE TABLE IF NOT EXISTS job_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  attempt_number integer NOT NULL,
  status text NOT NULL, -- succeeded | failed
  error text,
  started_at timestamptz NOT NULL,
  finished_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_job_attempts_job ON job_attempts(job_id, attempt_number);

COMMIT;
