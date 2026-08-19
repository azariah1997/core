BEGIN;

-- The scaffold's audit_events table (0001_core.sql) already covers
-- actor/action/resource/timestamp/correlation ID/application/metadata -
-- extended (confirmed empty) with the two fields it was missing:
-- tenant and device context, both explicitly "when appropriate" in the
-- roadmap, hence nullable.
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id);
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS device_id uuid REFERENCES devices(id);
CREATE INDEX IF NOT EXISTS ix_audit_actor ON audit_events(actor_user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS ix_audit_resource ON audit_events(resource_type, resource_id, occurred_at DESC);

-- "Audit must be immutable from normal application APIs" - the roadmap's
-- own explicit requirement. Enforced first by omission (this repo's
-- audit package has no Update/Delete method or route at all - the
-- strongest form of "can't misuse an API that doesn't exist"), and
-- reinforced here at the database level: even a bug, a future careless
-- migration, or a direct psql session under the shared application role
-- cannot mutate or remove a row once written.
CREATE OR REPLACE FUNCTION audit_events_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_events rows are immutable: % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_audit_events_immutable ON audit_events;
CREATE TRIGGER trg_audit_events_immutable
  BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

COMMIT;
