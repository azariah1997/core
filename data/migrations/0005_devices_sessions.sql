BEGIN;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_device_id text;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS os_version text;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS locale text;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS timezone text;
-- A device's own current session state (active/revoked). Not a separate
-- Session table yet: nothing in this phase needs more than one live
-- session per device, so that's the deferred extension, not this column.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS session_status text NOT NULL DEFAULT 'active';
CREATE UNIQUE INDEX IF NOT EXISTS ux_devices_user_client_device ON devices(user_id, client_device_id);
COMMIT;
