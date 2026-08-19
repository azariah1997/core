package billing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/billing"
	"github.com/example/core-platform/backend/core-api/internal/billing/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeProvider struct {
	name   string
	events map[string]billing.WebhookEvent // keyed by signatureHeader, a simple test stand-in
	err    error
}

func (p fakeProvider) Name() string { return p.name }
func (p fakeProvider) VerifyWebhook(ctx context.Context, payload []byte, sig string) (billing.WebhookEvent, error) {
	if p.err != nil {
		return billing.WebhookEvent{}, p.err
	}
	evt, ok := p.events[sig]
	if !ok {
		return billing.WebhookEvent{}, billing.ErrInvalidSignature
	}
	return evt, nil
}

func newService(admins map[string]bool) *billing.Service {
	return billing.NewService(memory.New(), fakeAdmin{admins: admins})
}

func TestHasEntitlementFalseBeforeGrant(t *testing.T) {
	svc := newService(nil)
	has, err := svc.HasEntitlement(context.Background(), "u1", "premium_tier")
	if err != nil {
		t.Fatalf("has entitlement: %v", err)
	}
	if has {
		t.Fatal("expected no entitlement before any grant")
	}
}

func TestGrantEntitlementRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.GrantEntitlement(ctx, "u1", billing.GrantEntitlementInput{UserID: "u2", Key: "premium_tier"}); !errors.Is(err, billing.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	e, err := svc.GrantEntitlement(ctx, "admin", billing.GrantEntitlementInput{UserID: "u2", Key: "premium_tier"})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if e.Source != "manual:admin" {
		t.Fatalf("expected source to record the granting admin, got %q", e.Source)
	}
	has, err := svc.HasEntitlement(ctx, "u2", "premium_tier")
	if err != nil || !has {
		t.Fatalf("expected u2 to now have the entitlement, got %v err=%v", has, err)
	}
}

func TestExpiredEntitlementIsNotActive(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := svc.GrantEntitlement(ctx, "admin", billing.GrantEntitlementInput{UserID: "u1", Key: "trial", ExpiresAt: &past}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	has, err := svc.HasEntitlement(ctx, "u1", "trial")
	if err != nil {
		t.Fatalf("has entitlement: %v", err)
	}
	if has {
		t.Fatal("expected an already-expired entitlement to be inactive")
	}
}

func TestRevokeEntitlementRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	e, err := svc.GrantEntitlement(ctx, "admin", billing.GrantEntitlementInput{UserID: "u1", Key: "premium_tier"})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svc.RevokeEntitlement(ctx, "u1", e.ID); !errors.Is(err, billing.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	if _, err := svc.RevokeEntitlement(ctx, "admin", e.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	has, err := svc.HasEntitlement(ctx, "u1", "premium_tier")
	if err != nil || has {
		t.Fatalf("expected the entitlement to no longer be active after revoke, got %v err=%v", has, err)
	}
}

func TestListEntitlementsRequiresOwnerOrAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.GrantEntitlement(ctx, "admin", billing.GrantEntitlementInput{UserID: "u1", Key: "premium_tier"}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svc.ListEntitlements(ctx, "stranger", "u1"); !errors.Is(err, billing.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger, got %v", err)
	}
	list, err := svc.ListEntitlements(ctx, "u1", "u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected the owner to list their own entitlement, got %v err=%v", list, err)
	}
}

func TestHandleWebhookGrantsEntitlementOnCompletedPayment(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	svc.RegisterProvider(fakeProvider{name: "stripe", events: map[string]billing.WebhookEvent{
		"sig1": {Type: billing.EventPaymentCompleted, ProviderRef: "cs_1", SubscriptionRef: "sub_1", UserID: "u1", EntitlementKey: "premium_tier", AmountCents: 999, Currency: "usd"},
	}})
	ctx := context.Background()
	if err := svc.HandleWebhook(ctx, "stripe", []byte(`{}`), "sig1"); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}
	has, err := svc.HasEntitlement(ctx, "u1", "premium_tier")
	if err != nil || !has {
		t.Fatalf("expected the completed payment to grant the entitlement, got %v err=%v", has, err)
	}
	payments, err := svc.ListPayments(ctx, "u1", "u1")
	if err != nil || len(payments) != 1 || payments[0].Status != billing.PaymentStatusCompleted {
		t.Fatalf("expected exactly one completed payment, got %+v err=%v", payments, err)
	}
}

func TestHandleWebhookIsIdempotentOnRedelivery(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	svc.RegisterProvider(fakeProvider{name: "stripe", events: map[string]billing.WebhookEvent{
		"sig1": {Type: billing.EventPaymentCompleted, ProviderRef: "cs_1", SubscriptionRef: "sub_1", UserID: "u1", EntitlementKey: "premium_tier", AmountCents: 999, Currency: "usd"},
	}})
	ctx := context.Background()
	if err := svc.HandleWebhook(ctx, "stripe", []byte(`{}`), "sig1"); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	// Stripe redelivers the identical event (e.g. this server's 2xx
	// response was lost in transit).
	if err := svc.HandleWebhook(ctx, "stripe", []byte(`{}`), "sig1"); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	payments, err := svc.ListPayments(ctx, "u1", "u1")
	if err != nil || len(payments) != 1 {
		t.Fatalf("expected exactly one payment despite two deliveries, got %d err=%v", len(payments), err)
	}
	entitlements, err := svc.ListEntitlements(ctx, "u1", "u1")
	if err != nil || len(entitlements) != 1 {
		t.Fatalf("expected exactly one entitlement despite two deliveries, got %d err=%v", len(entitlements), err)
	}
}

func TestHandleWebhookRevokesEntitlementOnSubscriptionCanceled(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	svc.RegisterProvider(fakeProvider{name: "stripe", events: map[string]billing.WebhookEvent{
		"completed": {Type: billing.EventPaymentCompleted, ProviderRef: "cs_1", SubscriptionRef: "sub_1", UserID: "u1", EntitlementKey: "premium_tier"},
		"canceled":  {Type: billing.EventSubscriptionCanceled, ProviderRef: "sub_1"},
	}})
	ctx := context.Background()
	if err := svc.HandleWebhook(ctx, "stripe", []byte(`{}`), "completed"); err != nil {
		t.Fatalf("completed: %v", err)
	}
	has, _ := svc.HasEntitlement(ctx, "u1", "premium_tier")
	if !has {
		t.Fatal("expected the entitlement to be active after the completed payment")
	}
	if err := svc.HandleWebhook(ctx, "stripe", []byte(`{}`), "canceled"); err != nil {
		t.Fatalf("canceled: %v", err)
	}
	has, err := svc.HasEntitlement(ctx, "u1", "premium_tier")
	if err != nil || has {
		t.Fatalf("expected the entitlement to be revoked after subscription cancellation, got %v err=%v", has, err)
	}
}

func TestHandleWebhookRejectsUnknownProvider(t *testing.T) {
	svc := newService(nil)
	err := svc.HandleWebhook(context.Background(), "apple_iap", []byte(`{}`), "sig")
	if !errors.Is(err, billing.ErrUnknownProvider) {
		t.Fatalf("expected ErrUnknownProvider, got %v", err)
	}
}

func TestHandleWebhookPropagatesVerificationFailure(t *testing.T) {
	svc := newService(nil)
	svc.RegisterProvider(fakeProvider{name: "stripe", err: billing.ErrInvalidSignature})
	err := svc.HandleWebhook(context.Background(), "stripe", []byte(`{}`), "bad-sig")
	if !errors.Is(err, billing.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature to propagate, got %v", err)
	}
}
