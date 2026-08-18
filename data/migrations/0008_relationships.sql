BEGIN;
ALTER TABLE relationships ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
COMMIT;
