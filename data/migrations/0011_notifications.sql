BEGIN;

-- The scaffold's "notifications" table already has exactly the shape of a
-- single-channel delivery attempt (category, channel, payload, status,
-- delivered_at) - not the logical, potentially multi-channel notification
-- a caller asks to send. Rather than rename it (breaking the "adapt to
-- what's already there" convention this repo has followed since
-- relationships/messaging), it becomes NotificationDelivery's table, and
-- the logical request gets a new table, notification_requests.
CREATE TABLE IF NOT EXISTS notification_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES applications(id),
  user_id uuid NOT NULL REFERENCES users(id),
  category text NOT NULL,
  title text NOT NULL,
  body text NOT NULL,
  data jsonb NOT NULL DEFAULT '{}'::jsonb,
  channels text[] NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_notification_requests_user ON notification_requests(user_id, created_at DESC);

-- notification_requests didn't exist when the "notifications" table was
-- created, hence the FK arrives via ALTER rather than at CREATE TABLE
-- time. Table is confirmed empty (no prior phase used it), so NOT NULL is
-- safe to add directly, same precedent as 0005_devices_sessions.sql.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS notification_request_id uuid REFERENCES notification_requests(id) ON DELETE CASCADE;
ALTER TABLE notifications ALTER COLUMN notification_request_id SET NOT NULL;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS provider_ref text;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS error text;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS ix_notifications_request ON notifications(notification_request_id);

CREATE TABLE IF NOT EXISTS notification_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES applications(id),
  -- Product-defined identifier (e.g. "message.new") - free-form, same
  -- convention as relationship_type/message_type/group role labels.
  key text NOT NULL,
  title_template text NOT NULL,
  body_template text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_notification_templates_app_key ON notification_templates(app_id, key);

-- Opt-out model: absence of a row means enabled. One row per
-- (user, app, category, channel) a user has explicitly toggled.
CREATE TABLE IF NOT EXISTS notification_preferences (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  app_id uuid NOT NULL REFERENCES applications(id),
  category text NOT NULL,
  channel text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  PRIMARY KEY(user_id, app_id, category, channel)
);

-- start_minute/end_minute are minutes-since-local-midnight; a window
-- where start > end (e.g. 22:00-07:00) wraps past midnight - handled in
-- Go, not the schema.
CREATE TABLE IF NOT EXISTS notification_quiet_hours (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  app_id uuid NOT NULL REFERENCES applications(id),
  timezone text NOT NULL DEFAULT 'UTC',
  start_minute integer NOT NULL CHECK (start_minute >= 0 AND start_minute < 1440),
  end_minute integer NOT NULL CHECK (end_minute >= 0 AND end_minute < 1440),
  enabled boolean NOT NULL DEFAULT true,
  PRIMARY KEY(user_id, app_id)
);

COMMIT;
