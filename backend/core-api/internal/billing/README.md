# Billing / Entitlements

The roadmap's own framing for this phase: separate `Payment` from `Entitlement`. "Products should ask: user has entitlement X? not: is the Stripe subscription active?" "Implement provider abstraction first."

## Payment vs. Entitlement

`Entitlement` is the platform-facing truth - `Service.HasEntitlement(ctx, userID, key)` is the literal question the roadmap says products should ask, and the only thing most product code should ever call. `Payment` is a provider-specific transaction record, kept deliberately separate: a manually-granted entitlement (comps, support gestures) has no payment at all, and a refunded payment doesn't automatically mean the entitlement it originally granted should vanish immediately - those are two different lifecycles this repo doesn't collapse into one table.

`Entitlement.IsActive` is computed at query time (not revoked, and either no expiry or still before it) rather than a stored, scheduler-maintained flag - the same "don't invent scheduler infrastructure this phase doesn't need" principle Phase 21 applies to Suspension/Ban and Phase 13 applies to file retention.

## Provider abstraction

```go
type PaymentProvider interface {
    Name() string
    VerifyWebhook(ctx context.Context, payload []byte, signatureHeader string) (WebhookEvent, error)
}
```

is the entire surface - "implement provider abstraction first" satisfied literally. `Service.RegisterProvider` is a registry (the same pattern Phase 20's privacy Exporter/Deleter registration established): adding a second provider later is one new adapter package plus one line in `cmd/server/main.go`, never a change to this package.

### Stripe: the one real, live-testable implementation

`billing/stripe` implements Stripe's actual published webhook signature scheme - HMAC-SHA256 over `"{timestamp}.{payload}"`, keyed by the webhook's shared secret, with a 5-minute replay-tolerance window (Stripe's own SDK default). This is a pure cryptographic function with no network dependency, so it's the one provider this phase could rigorously validate **without a real Stripe account**: self-generate a signed test payload exactly the way Stripe's servers would, and confirm the verification logic genuinely accepts valid signatures and rejects tampered payloads, wrong secrets, and expired timestamps - both as unit tests (`stripe/stripe_test.go`) and live, against the real running server (see Validation below).

Two Stripe event types are mapped: `checkout.session.completed` (grants the entitlement named in the session's `metadata.entitlementKey`, for the user in `metadata.userId`) and `customer.subscription.deleted` (revokes it). Passing `userId`/`entitlementKey` via Checkout Session metadata is a real, commonly-used Stripe integration pattern - the alternative (a separate Stripe-customer-ID-to-platform-user-ID mapping table) would require a full Checkout Session creation flow this phase doesn't build.

### Apple IAP / Google Play: deliberately not implemented

Both need real, provider-specific receipt/notification verification against Apple's App Store Server API or Google's Play Developer API - actual network calls requiring real app-specific sandbox credentials this environment doesn't have. Faking their verification would mean pretending an unverified receipt is trustworthy, which is worse than not implementing them at all. The `PaymentProvider` interface already accommodates them with zero changes needed here; adding either later is exactly the same shape of work `billing/stripe` already demonstrates.

## Idempotency

Stripe explicitly retries webhook delivery on anything but a 2xx response, and can occasionally redeliver even after a successful one. Both event types this phase handles are idempotent by construction, not by accident:

- **Payment recording**: `payments` has a `UNIQUE (provider, provider_ref)` constraint; `RecordPayment` does `INSERT ... ON CONFLICT DO NOTHING RETURNING ...` - a redelivery's insert is skipped, `RETURNING` yields zero rows, and `Service.HandleWebhook` reads that as "already processed, don't grant again." Confirmed live: sending the identical signed payload twice produced exactly one payment row and did not grant a second entitlement.
- **Subscription cancellation**: `RevokeBySource` only ever updates still-active rows (`WHERE revoked_at IS NULL`) - revoking an already-revoked entitlement a second time affects zero rows rather than erroring.

## The one route with no Bearer token

`POST /v1/billing/webhooks/{provider}` is registered separately from every other route in this platform (`RegisterWebhookRoute`, not `RegisterRoutes`) and deliberately carries **no** `requireUser`/`requireActive` wrapper - a payment provider has no platform account or Bearer token. Its identity is the webhook signature itself, verified inside `PaymentProvider.VerifyWebhook` - the same trust boundary Stripe, Apple, and Google themselves use for server-to-server delivery. The handler reads the raw request body before any JSON decoding, since signature verification must run over the exact bytes the provider signed.

## Manual grants

`POST /v1/billing/entitlements` is platform.admin-only, for the case no `PaymentProvider` handles: comps, support gestures, migrations. `Source` is always written as `"manual:<callerID>"` server-side, never accepted from the request body, so a grant's provenance can never be spoofed to look like it came from a real payment.

## Layout

- `domain.go` - `Entitlement`, `Payment`, `WebhookEvent`, `PaymentProvider`, `Repository`.
- `service.go` - `Service`, the provider registry, `HasEntitlement`, `HandleWebhook`.
- `http.go` - the REST surface (5 authenticated routes) plus the separately-registered webhook route.
- `postgres/`, `memory/` - `Repository` implementations.
- `stripe/` - the one real `PaymentProvider` this phase ships.

## Storage

`entitlements`, `payments` (`data/migrations/0020_billing.sql`) - both new tables, no pre-existing scaffold.

## Not done here

No outbox event is emitted when an entitlement is granted or revoked - a plausible future consumer exists (notifications could welcome a new subscriber, analytics could track conversion), but nothing in this platform needs it yet, and speculative events with no real consumer aren't this repo's style; `contracts/asyncapi/events.yaml` is unchanged this phase, confirmed rather than silently skipped. `Payment.Status` includes `refunded` in its type but no code path sets it yet - no Stripe event this phase maps produces a refund; a real follow-up, not solved here.
