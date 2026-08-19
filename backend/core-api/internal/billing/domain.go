// Package billing implements Phase 22: separating Payment from
// Entitlement. "Products should ask: user has entitlement X? not: is
// the Stripe subscription active?" is the roadmap's own framing, and
// the reason Entitlement (this package's platform-facing truth) and
// Payment (a provider-specific transaction record) are two different
// types rather than one - a manually-granted entitlement has no
// payment at all, and a refunded payment doesn't necessarily mean the
// entitlement it originally granted should vanish immediately.
//
// "Implement provider abstraction first" is satisfied by
// PaymentProvider (service.go) - see billing/stripe for the one real,
// live-testable implementation this phase ships, and this package's
// README for why Apple IAP/Google Play are deliberately not.
package billing

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Entitlement is computed-active, not a stored flag - the same
// "IsActive at query time, not scheduler-maintained" principle Phase 21
// applies to Suspension/Ban: a scheduler flipping expired entitlements
// to inactive is infrastructure this phase's scope doesn't need.
type Entitlement struct {
	ID        string
	UserID    string
	Key       string
	Source    string
	GrantedAt time.Time
	ExpiresAt *time.Time
	RevokedAt *time.Time
}

func (e Entitlement) IsActive(now time.Time) bool {
	if e.RevokedAt != nil {
		return false
	}
	return e.ExpiresAt == nil || now.Before(*e.ExpiresAt)
}

type GrantEntitlementInput struct {
	UserID    string
	Key       string
	Source    string
	ExpiresAt *time.Time
}

func (in GrantEntitlementInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return &ValidationError{"userId is required"}
	}
	if strings.TrimSpace(in.Key) == "" {
		return &ValidationError{"key is required"}
	}
	return nil
}

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

type Payment struct {
	ID          string
	UserID      string
	Provider    string
	ProviderRef string
	AmountCents int64
	Currency    string
	Status      PaymentStatus
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WebhookEventType is this package's own vocabulary, translated from
// whatever wire format a given PaymentProvider uses - callers of
// Service.HandleWebhook depend on this contract, never a provider's
// raw event types, the same layering as every other module wrapping an
// external system in this repo (e.g. workflows.Status over Temporal's).
type WebhookEventType string

const (
	EventPaymentCompleted     WebhookEventType = "payment.completed"
	EventPaymentFailed        WebhookEventType = "payment.failed"
	EventSubscriptionCanceled WebhookEventType = "subscription.canceled"
)

// WebhookEvent is what a PaymentProvider hands back after verifying a
// webhook - ProviderRef is the dedup key for this specific event
// (a checkout session ID for a completed payment; a subscription ID for
// a cancellation), while SubscriptionRef (only set on a completed
// payment) is what a later cancellation event's ProviderRef will match
// against an Entitlement's Source to find what to revoke.
type WebhookEvent struct {
	Type            WebhookEventType
	ProviderRef     string
	SubscriptionRef string
	UserID          string
	EntitlementKey  string
	AmountCents     int64
	Currency        string
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound         = errors.New("resource not found")
	ErrForbidden        = errors.New("not permitted to perform this billing action")
	ErrUnknownProvider  = errors.New("unknown payment provider")
	ErrInvalidSignature = errors.New("invalid webhook signature")
	ErrSignatureExpired = errors.New("webhook signature timestamp outside tolerance")
	ErrUnhandledEvent   = errors.New("unhandled webhook event type")
)

// Repository is the storage-agnostic boundary.
type Repository interface {
	GrantEntitlement(ctx context.Context, e Entitlement) error
	RevokeEntitlement(ctx context.Context, id string) (Entitlement, error)
	// RevokeBySource revokes every currently-active entitlement whose
	// Source matches (e.g. "stripe:sub_123") - how a subscription
	// cancellation event revokes without needing to know individual
	// entitlement IDs. Idempotent: revoking an already-revoked row a
	// second time (a redelivered cancellation webhook) affects zero
	// rows, not an error.
	RevokeBySource(ctx context.Context, source string) error
	ListEntitlements(ctx context.Context, userID string) ([]Entitlement, error)
	GetEntitlement(ctx context.Context, id string) (Entitlement, error)

	// RecordPayment returns (payment, created) - created is false when
	// (provider, providerRef) already existed (a redelivered webhook),
	// letting Service skip re-granting an entitlement for a payment it
	// already processed.
	RecordPayment(ctx context.Context, p Payment) (Payment, bool, error)
	ListPayments(ctx context.Context, userID string) ([]Payment, error)
}
