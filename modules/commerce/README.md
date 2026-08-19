# Commerce module

Owns the **commerce** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/billing` - `Entitlement` (the platform-facing truth, `HasEntitlement`) kept deliberately separate from `Payment` (a provider-specific transaction record), behind a `PaymentProvider` interface + registry. `billing/stripe` implements Stripe's real webhook HMAC-SHA256 signature scheme, live-validated without a Stripe account via self-signed test payloads - the one genuinely live-testable provider this environment has credentials for. Apple IAP/Google Play are structurally supported but not implemented, the same "no real sandbox credentials" reasoning as the AI Gateway's unimplemented vendor adapters. Exposed at `/v1/billing/*`. See that package's README for detail.
