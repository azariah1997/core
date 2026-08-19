BEGIN;

-- Block already exists (Phase 8's relationships, status='blocked') -
-- deliberately not duplicated here. Mute is new: one-directional and
-- silent (the muted user is never notified and isn't prevented from
-- interacting - only the muter stops seeing them), unlike Block which
-- is a full relationship state change both sides can observe.
CREATE TABLE IF NOT EXISTS mutes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  muter_user_id uuid NOT NULL REFERENCES users(id),
  muted_user_id uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (muter_user_id, muted_user_id)
);

-- One current review record per (resource_type, resource_id) WHILE
-- open - the partial unique index below means a new report against a
-- resource that already has an open case attaches to it instead of
-- opening a duplicate, but a resource can get a fresh case later if a
-- prior one was already resolved/dismissed (renewed abuse deserves a
-- new look, not silence because the old case is closed).
CREATE TABLE IF NOT EXISTS moderation_cases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  status text NOT NULL DEFAULT 'open',
  assigned_to uuid REFERENCES users(id),
  resolution text,
  report_count integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_moderation_cases_open_resource
  ON moderation_cases(resource_type, resource_id) WHERE status IN ('open', 'in_review');
CREATE INDEX IF NOT EXISTS ix_moderation_cases_status ON moderation_cases(status, created_at DESC);

-- Reason is a free-form, product-defined string - "design so products
-- can supply product-specific report reasons" is the roadmap's own
-- instruction, the same convention as RelationshipType (Phase 8) and
-- Message.Type (Phase 11): never a fixed platform enum.
CREATE TABLE IF NOT EXISTS reports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  reporter_user_id uuid NOT NULL REFERENCES users(id),
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  reason text NOT NULL,
  details text,
  case_id uuid NOT NULL REFERENCES moderation_cases(id),
  status text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_reports_case ON reports(case_id);
CREATE INDEX IF NOT EXISTS ix_reports_reporter ON reports(reporter_user_id, created_at DESC);

-- Suspension is temporary (ends_at required); Ban has no ends_at at
-- all - permanent until explicitly lifted. Both use lifted_at/lifted_by
-- rather than a separately-maintained "active" boolean that would need
-- a scheduler to flip when ends_at passes - IsActive is computed at
-- query time instead (see service.go).
CREATE TABLE IF NOT EXISTS suspensions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  reason text NOT NULL,
  case_id uuid REFERENCES moderation_cases(id),
  issued_by uuid NOT NULL REFERENCES users(id),
  starts_at timestamptz NOT NULL DEFAULT now(),
  ends_at timestamptz NOT NULL,
  lifted_at timestamptz,
  lifted_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_suspensions_user ON suspensions(user_id, ends_at DESC);

CREATE TABLE IF NOT EXISTS bans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  reason text NOT NULL,
  case_id uuid REFERENCES moderation_cases(id),
  issued_by uuid NOT NULL REFERENCES users(id),
  lifted_at timestamptz,
  lifted_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_bans_user ON bans(user_id, created_at DESC);

-- Appeal targets a suspension or a ban by ID - a free-form target_type
-- string rather than two separate appeal tables, since the review flow
-- (pending/approved/denied, who reviewed it, lifting the target on
-- approval) is identical either way.
CREATE TABLE IF NOT EXISTS appeals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  target_type text NOT NULL,
  target_id uuid NOT NULL,
  reason text NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  reviewed_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz
);
CREATE INDEX IF NOT EXISTS ix_appeals_user ON appeals(user_id, created_at DESC);

-- AbuseSignal is a lightweight, open-to-record observation - distinct
-- from Report (a person reporting something) and from Phase 19's audit
-- (a record of what happened): a signal is "something looks off about
-- X," which may or may not warrant a human ever looking at it. Recorded
-- freely; only a "critical" one auto-opens a ModerationCase (see
-- service.go) - a deliberately simple escalation rule, not a full
-- abuse-detection engine, which is out of this phase's scope.
CREATE TABLE IF NOT EXISTS abuse_signals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  signal_type text NOT NULL,
  severity text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_abuse_signals_resource ON abuse_signals(resource_type, resource_id, recorded_at DESC);

COMMIT;
