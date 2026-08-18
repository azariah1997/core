BEGIN;

-- The scaffold's "files" table already covers ownership/mime/size/
-- visibility - extended (table confirmed empty) rather than replaced,
-- same pattern as every prior phase's pre-existing-table adaptation.
ALTER TABLE files ADD COLUMN IF NOT EXISTS file_name text NOT NULL DEFAULT '';
ALTER TABLE files ALTER COLUMN file_name DROP DEFAULT;
-- pending: upload URL issued, bytes not yet confirmed in storage.
-- active: confirmed present in storage with verified size/checksum.
-- deleted: soft-deleted - object removed from storage, row kept for audit.
ALTER TABLE files ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'pending';
ALTER TABLE files ADD COLUMN IF NOT EXISTS checksum text;
ALTER TABLE files ADD COLUMN IF NOT EXISTS expires_at timestamptz;
ALTER TABLE files ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS ix_files_owner ON files(owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_files_expiry ON files(expires_at) WHERE status = 'active' AND expires_at IS NOT NULL;

COMMIT;
