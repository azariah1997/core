BEGIN;

-- Fully new tables, no pre-existing scaffold, same as jobs (0013) and
-- workflow_runs (0014).
CREATE TABLE IF NOT EXISTS features (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES applications(id),
  key text NOT NULL, -- product-defined, e.g. "new-checkout-flow"
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  -- Master kill-switch: false always evaluates the feature off, before
  -- any rule is even considered.
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_features_app_key ON features(app_id, key);

-- One row per targeting rule. `conditions` holds every optional
-- dimension (environment/user/tenant/platform/country/version/
-- percentage) as a single jsonb object rather than a dozen nullable
-- columns or join tables - each dimension is independently optional
-- ("don't care" when absent), and jsonb lets a rule combine any subset
-- of them without a schema change per new dimension.
CREATE TABLE IF NOT EXISTS feature_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  feature_id uuid NOT NULL REFERENCES features(id) ON DELETE CASCADE,
  -- Rules are evaluated in ascending priority order; the first whose
  -- conditions all match wins (first-match-wins, the same semantics as
  -- most real feature-flag systems).
  priority integer NOT NULL DEFAULT 0,
  conditions jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- What the feature evaluates to when this rule matches - usually true,
  -- but false lets an earlier, higher-priority rule explicitly exclude a
  -- segment before a later, broader rule would otherwise include it.
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_feature_rules_feature ON feature_rules(feature_id, priority);

COMMIT;
