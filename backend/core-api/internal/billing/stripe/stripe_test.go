package stripe_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/billing"
	"github.com/example/core-platform/backend/core-api/internal/billing/stripe"
)

const testSecret = "whsec_test_secret_do_not_use_in_production"

// sign reproduces exactly what a real Stripe webhook sender does -
// used here to generate genuine test payloads, not a shortcut around
// the real algorithm.
func sign(secret, timestamp, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func header(secret, payload string, ts time.Time) string {
	t := fmt.Sprintf("%d", ts.Unix())
	return "t=" + t + ",v1=" + sign(secret, t, payload)
}

func TestValidSignatureIsAccepted(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	payload := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","amount_total":999,"currency":"usd","subscription":"sub_1","metadata":{"userId":"u1","entitlementKey":"premium_tier"}}}}`
	sig := header(testSecret, payload, time.Now())

	evt, err := p.VerifyWebhook(context.Background(), []byte(payload), sig)
	if err != nil {
		t.Fatalf("expected a genuinely valid signature to verify, got %v", err)
	}
	if evt.Type != billing.EventPaymentCompleted || evt.UserID != "u1" || evt.EntitlementKey != "premium_tier" || evt.SubscriptionRef != "sub_1" || evt.ProviderRef != "cs_test_1" || evt.AmountCents != 999 {
		t.Fatalf("unexpected parsed event: %+v", evt)
	}
}

func TestTamperedPayloadIsRejected(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	payload := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","amount_total":999,"currency":"usd","metadata":{"userId":"u1","entitlementKey":"premium_tier"}}}}`
	sig := header(testSecret, payload, time.Now())

	// An attacker who intercepted a real webhook and changed the
	// amount, without knowing the secret, cannot produce a signature
	// that still matches - the same tamper-evidence property real
	// Stripe verification provides.
	tampered := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","amount_total":999999,"currency":"usd","metadata":{"userId":"u1","entitlementKey":"premium_tier"}}}}`

	_, err := p.VerifyWebhook(context.Background(), []byte(tampered), sig)
	if !errors.Is(err, billing.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for a tampered payload, got %v", err)
	}
}

func TestWrongSecretIsRejected(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	payload := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","metadata":{"userId":"u1"}}}}`
	sig := header("whsec_a_completely_different_secret", payload, time.Now())

	_, err := p.VerifyWebhook(context.Background(), []byte(payload), sig)
	if !errors.Is(err, billing.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature when signed with the wrong secret, got %v", err)
	}
}

func TestExpiredTimestampIsRejected(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret, Tolerance: 5 * time.Minute})
	payload := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","metadata":{"userId":"u1"}}}}`
	sig := header(testSecret, payload, time.Now().Add(-10*time.Minute))

	_, err := p.VerifyWebhook(context.Background(), []byte(payload), sig)
	if !errors.Is(err, billing.ErrSignatureExpired) {
		t.Fatalf("expected ErrSignatureExpired for a 10-minute-old signature under a 5-minute tolerance, got %v", err)
	}
}

func TestMalformedHeaderIsRejected(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	_, err := p.VerifyWebhook(context.Background(), []byte(`{}`), "not-a-valid-header")
	if !errors.Is(err, billing.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for a malformed header, got %v", err)
	}
}

func TestSubscriptionDeletedEventParses(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	payload := `{"type":"customer.subscription.deleted","data":{"object":{"id":"sub_1"}}}`
	sig := header(testSecret, payload, time.Now())

	evt, err := p.VerifyWebhook(context.Background(), []byte(payload), sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if evt.Type != billing.EventSubscriptionCanceled || evt.ProviderRef != "sub_1" {
		t.Fatalf("unexpected event: %+v", evt)
	}
}

func TestUnhandledEventTypeIsRejected(t *testing.T) {
	p := stripe.New(stripe.Config{WebhookSecret: testSecret})
	payload := `{"type":"charge.dispute.created","data":{"object":{"id":"dp_1"}}}`
	sig := header(testSecret, payload, time.Now())

	_, err := p.VerifyWebhook(context.Background(), []byte(payload), sig)
	if !errors.Is(err, billing.ErrUnhandledEvent) {
		t.Fatalf("expected ErrUnhandledEvent, got %v", err)
	}
}
