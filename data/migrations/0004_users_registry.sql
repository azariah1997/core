BEGIN;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_ref text;
-- identity_subject was the pre-Phase-3 way of linking a user to one
-- external identity 1:1. It's superseded by identities.user_id (which
-- supports multiple linked identities per user) but left in place rather
-- than dropped - no reader depends on it, and dropping a column is a
-- non-additive change this repo's migration rules avoid without cause.
-- New rows created through the Phase 4 API leave it NULL.
ALTER TABLE users ALTER COLUMN identity_subject DROP NOT NULL;
COMMIT;
