// Package stripe implements billing.PaymentProvider against Stripe's
// real webhook signature scheme - the one PaymentProvider this phase
// ships as genuinely live-testable without a real Stripe account.
// Signature verification is a pure cryptographic function (HMAC-SHA256
// over "timestamp.payload"), documented publicly by Stripe and
// independent of any network call, so it can be exercised end-to-end
// with a self-signed test payload exactly the way a real Stripe webhook
// would arrive - see this package's tests and billing/README.md for
// how that was verified live, not just unit-tested.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/billing"
)

type Config struct {
	WebhookSecret string
	// Tolerance defaults to 5 minutes, Stripe's own SDK default -
	// rejects a signature whose timestamp is older than this, the
	// standard defense against a captured webhook payload being replayed
	// later.
	Tolerance time.Duration
}

type Provider struct {
	secret    string
	tolerance time.Duration
}

func New(cfg Config) *Provider {
	tolerance := cfg.Tolerance
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	return &Provider{secret: cfg.WebhookSecret, tolerance: tolerance}
}

func (p *Provider) Name() string { return "stripe" }

// event is the small subset of Stripe's real event envelope this
// provider actually reads - userId/entitlementKey travel in Checkout
// Session metadata, a real, commonly-used Stripe integration pattern
// (rather than this platform maintaining its own Stripe-customer-ID-to-
// platform-user-ID mapping table, which a full Stripe Customer/Checkout
// integration this phase doesn't build would otherwise require).
type event struct {
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID           string            `json:"id"`
			AmountTotal  int64             `json:"amount_total"`
			Currency     string            `json:"currency"`
			Subscription string            `json:"subscription"`
			Metadata     map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

func (p *Provider) VerifyWebhook(ctx context.Context, payload []byte, signatureHeader string) (billing.WebhookEvent, error) {
	ts, sig, err := parseSignatureHeader(signatureHeader)
	if err != nil {
		return billing.WebhookEvent{}, billing.ErrInvalidSignature
	}

	mac := hmac.New(sha256.New, []byte(p.secret))
	mac.Write([]byte(ts + "." + string(payload)))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return billing.WebhookEvent{}, billing.ErrInvalidSignature
	}

	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return billing.WebhookEvent{}, billing.ErrInvalidSignature
	}
	age := time.Since(time.Unix(tsInt, 0))
	if age < 0 {
		age = -age
	}
	if age > p.tolerance {
		return billing.WebhookEvent{}, billing.ErrSignatureExpired
	}

	var evt event
	if err := json.Unmarshal(payload, &evt); err != nil {
		return billing.WebhookEvent{}, fmt.Errorf("parse stripe webhook payload: %w", err)
	}

	switch evt.Type {
	case "checkout.session.completed":
		return billing.WebhookEvent{
			Type: billing.EventPaymentCompleted, ProviderRef: evt.Data.Object.ID, SubscriptionRef: evt.Data.Object.Subscription,
			UserID: evt.Data.Object.Metadata["userId"], EntitlementKey: evt.Data.Object.Metadata["entitlementKey"],
			AmountCents: evt.Data.Object.AmountTotal, Currency: evt.Data.Object.Currency,
		}, nil
	case "customer.subscription.deleted":
		return billing.WebhookEvent{Type: billing.EventSubscriptionCanceled, ProviderRef: evt.Data.Object.ID}, nil
	default:
		return billing.WebhookEvent{}, billing.ErrUnhandledEvent
	}
}

// parseSignatureHeader parses Stripe's "t=<unix ts>,v1=<hex hmac>[,v1=<hex hmac>...]"
// format, returning the timestamp and the first v1 signature - Stripe
// sends multiple v1 values only during secret rotation, which this
// phase's single-secret Config doesn't need to support.
func parseSignatureHeader(header string) (timestamp, signature string, err error) {
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			if signature == "" {
				signature = kv[1]
			}
		}
	}
	if timestamp == "" || signature == "" {
		return "", "", fmt.Errorf("malformed Stripe-Signature header")
	}
	return timestamp, signature, nil
}
