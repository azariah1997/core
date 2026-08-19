BEGIN;

-- Entitlement is the platform-facing truth products ask about ("does
-- this user have entitlement X") - never "is the Stripe subscription
-- active," per the roadmap's own explicit framing. Key is a free-form,
-- product-defined string (e.g. "premium_tier", "ad_free"), the same
-- convention as every other Type/Key-shaped field in this repo. Source
-- records where the grant came from ("stripe:sub_123" or
-- "manual:<adminUserId>") without this table needing a foreign key into
-- payments - a manual grant has no payment at all.
CREATE TABLE IF NOT EXISTS entitlements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  key text NOT NULL,
  source text NOT NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS ix_entitlements_user ON entitlements(user_id, key);
-- Used to find "every entitlement this subscription granted" when a
-- provider tells us the subscription was canceled - see
-- billing/postgres/repository.go's RevokeBySource.
CREATE INDEX IF NOT EXISTS ix_entitlements_source ON entitlements(source);

-- Payment is the provider-specific transaction record - deliberately
-- separate from Entitlement, the whole point of this phase. provider is
-- free-form ("stripe", "apple_iap", "google_play" - the roadmap's own
-- "may include" list, not a fixed enum this table enforces).
-- (provider, provider_ref) is unique so a redelivered webhook (Stripe
-- explicitly retries on anything but a 2xx, and can occasionally
-- duplicate even on success) can be inserted idempotently via
-- ON CONFLICT DO NOTHING rather than double-recording the same payment.
CREATE TABLE IF NOT EXISTS payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  provider text NOT NULL,
  provider_ref text NOT NULL,
  amount_cents bigint NOT NULL,
  currency text NOT NULL,
  status text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_ref)
);
CREATE INDEX IF NOT EXISTS ix_payments_user ON payments(user_id, created_at DESC);

COMMIT;
