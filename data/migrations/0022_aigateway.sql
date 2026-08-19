BEGIN;

-- One row per completion call - this is simultaneously this phase's
-- token usage record, cost tracking record, and (via a companion
-- audit_events row - see aigateway/README.md) audit trail. Unlike
-- Phase 23's analytics_events (a landing buffer nothing queries
-- directly), this table IS meant to be read back directly by its own
-- owner - "how many completions have I made, what did they cost" is a
-- legitimate, narrow, self-service question, not a general analytics
-- query.
CREATE TABLE IF NOT EXISTS ai_completions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid REFERENCES users(id),
  app_id uuid REFERENCES applications(id),
  model_alias text NOT NULL,
  provider text NOT NULL,
  model text NOT NULL,
  prompt_key text,
  prompt_version text,
  prompt_tokens integer NOT NULL DEFAULT 0,
  completion_tokens integer NOT NULL DEFAULT 0,
  total_tokens integer NOT NULL DEFAULT 0,
  cost_cents numeric(12, 6) NOT NULL DEFAULT 0,
  latency_ms integer NOT NULL DEFAULT 0,
  finish_reason text,
  status text NOT NULL,
  error text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_ai_completions_user ON ai_completions(user_id, created_at DESC);

COMMIT;
