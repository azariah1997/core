BEGIN;

-- "Operational databases must not become analytics databases" is the
-- roadmap's own explicit constraint. This table is deliberately NOT a
-- queryable analytics store - it's a short-lived landing buffer, the
-- same role outbox_events (0001_core.sql) plays for domain events: rows
-- land here, a worker pipeline flushes them out in batches, and
-- flushed_at marks what's already shipped. Nothing in this platform
-- ever runs an analytical query (aggregation, funnel, cohort) against
-- this table - that's what the "pipeline foundation for future
-- ClickHouse/warehouse/data lake" this phase builds is for.
CREATE TABLE IF NOT EXISTS analytics_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_name text NOT NULL,
  user_id uuid REFERENCES users(id),
  anonymous_id text,
  app_id uuid REFERENCES applications(id),
  session_id text,
  occurred_at timestamptz NOT NULL,
  properties jsonb NOT NULL DEFAULT '{}'::jsonb,
  context jsonb NOT NULL DEFAULT '{}'::jsonb,
  ingested_at timestamptz NOT NULL DEFAULT now(),
  flushed_at timestamptz,
  batch_ref text
);

-- The pipeline's own claiming query filters on this - a partial index
-- keeps it cheap even once millions of already-flushed rows accumulate,
-- the same technique Phase 15's job queue and Phase 14's outbox
-- consumer both rely on.
CREATE INDEX IF NOT EXISTS ix_analytics_events_unflushed ON analytics_events(ingested_at) WHERE flushed_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_analytics_events_app ON analytics_events(app_id, occurred_at DESC);

COMMIT;
