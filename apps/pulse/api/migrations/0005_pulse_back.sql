-- Links a Pulse Back (product spec §17) to the original interaction it
-- reciprocates. NULL for an ordinary Pulse. References interactions.id
-- by value only (no FK constraint needed within the same table for
-- this - a self-reference FK would complicate the idempotency-retry
-- INSERT path for no real benefit, since the app layer already
-- guarantees the referenced row exists before creating the Pulse Back).
ALTER TABLE interactions ADD COLUMN IF NOT EXISTS in_response_to_id TEXT;

CREATE INDEX IF NOT EXISTS ix_interactions_in_response_to ON interactions (in_response_to_id) WHERE in_response_to_id IS NOT NULL;
