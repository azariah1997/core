package billing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// PaymentProvider is "implement provider abstraction first," the
// roadmap's own explicit instruction for this phase. VerifyWebhook is
// the entire surface: confirm payload+signature genuinely came from
// this provider, and translate it into this package's own WebhookEvent
// - nothing outside a provider's own implementation package (e.g.
// billing/stripe) ever sees a provider's wire format or SDK types, the
// same "do not expose the external system directly" principle Phase 16
// applies to Temporal and Phase 3 applies to Keycloak.
type PaymentProvider interface {
	Name() string
	VerifyWebhook(ctx context.Context, payload []byte, signatureHeader string) (WebhookEvent, error)
}

type Service struct {
	repo      Repository
	admin     AdminChecker
	providers map[string]PaymentProvider
}

func NewService(repo Repository, admin AdminChecker) *Service {
	return &Service{repo: repo, admin: admin, providers: map[string]PaymentProvider{}}
}

// RegisterProvider is how a concrete adapter (billing/stripe today;
// billing/apple or billing/google could be added later with zero
// changes here) opts into HandleWebhook - called once per provider from
// cmd/server/main.go, the same registry pattern Phase 20's privacy
// Exporter/Deleter registration established.
func (s *Service) RegisterProvider(p PaymentProvider) {
	s.providers[p.Name()] = p
}

// HasEntitlement is the literal question the roadmap says products
// should ask instead of checking a provider's subscription status
// directly.
func (s *Service) HasEntitlement(ctx context.Context, userID, key string) (bool, error) {
	list, err := s.repo.ListEntitlements(ctx, userID)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	for _, e := range list {
		if e.Key == key && e.IsActive(now) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) ListEntitlements(ctx context.Context, callerID, targetUserID string) ([]Entitlement, error) {
	if err := s.requireOwnerOrAdmin(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	return s.repo.ListEntitlements(ctx, targetUserID)
}

// GrantEntitlement is platform.admin-only - the manual-grant escape
// hatch (comps, support gestures, migrations) alongside whatever a
// PaymentProvider's webhook grants automatically. Source is always
// "manual:<callerID>" here, never caller-supplied, so a grant's
// provenance can't be spoofed to look like it came from a real payment.
func (s *Service) GrantEntitlement(ctx context.Context, callerID string, in GrantEntitlementInput) (Entitlement, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Entitlement{}, err
	}
	if err := in.Validate(); err != nil {
		return Entitlement{}, err
	}
	e := Entitlement{
		ID: uuid.NewString(), UserID: in.UserID, Key: in.Key, Source: "manual:" + callerID,
		GrantedAt: time.Now().UTC(), ExpiresAt: in.ExpiresAt,
	}
	if err := s.repo.GrantEntitlement(ctx, e); err != nil {
		return Entitlement{}, err
	}
	return e, nil
}

func (s *Service) RevokeEntitlement(ctx context.Context, callerID, id string) (Entitlement, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Entitlement{}, err
	}
	return s.repo.RevokeEntitlement(ctx, id)
}

func (s *Service) ListPayments(ctx context.Context, callerID, targetUserID string) ([]Payment, error) {
	if err := s.requireOwnerOrAdmin(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	return s.repo.ListPayments(ctx, targetUserID)
}

// HandleWebhook is the one route in this entire platform that doesn't
// authenticate via a Bearer token - it authenticates via the provider's
// own webhook signature instead (see http.go), the same trust boundary
// Stripe/Apple/Google themselves use for server-to-server delivery.
// Idempotent by construction: RecordPayment's (provider, providerRef)
// uniqueness means a redelivered "completed" webhook is recorded once,
// and created=false on the redelivery skips granting a second,
// duplicate entitlement; RevokeBySource is naturally idempotent since
// revoking an already-revoked row is a no-op.
func (s *Service) HandleWebhook(ctx context.Context, providerName string, payload []byte, signatureHeader string) error {
	p, ok := s.providers[providerName]
	if !ok {
		return ErrUnknownProvider
	}
	event, err := p.VerifyWebhook(ctx, payload, signatureHeader)
	if err != nil {
		return err
	}

	switch event.Type {
	case EventPaymentCompleted:
		payment := Payment{
			ID: uuid.NewString(), UserID: event.UserID, Provider: providerName, ProviderRef: event.ProviderRef,
			AmountCents: event.AmountCents, Currency: event.Currency, Status: PaymentStatusCompleted,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		_, created, err := s.repo.RecordPayment(ctx, payment)
		if err != nil {
			return err
		}
		if !created {
			return nil // already processed this exact payment - do not grant twice
		}
		source := providerName
		if event.SubscriptionRef != "" {
			source += ":" + event.SubscriptionRef
		} else {
			source += ":" + event.ProviderRef
		}
		return s.repo.GrantEntitlement(ctx, Entitlement{
			ID: uuid.NewString(), UserID: event.UserID, Key: event.EntitlementKey, Source: source, GrantedAt: time.Now().UTC(),
		})

	case EventPaymentFailed:
		payment := Payment{
			ID: uuid.NewString(), UserID: event.UserID, Provider: providerName, ProviderRef: event.ProviderRef,
			AmountCents: event.AmountCents, Currency: event.Currency, Status: PaymentStatusFailed,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		_, _, err := s.repo.RecordPayment(ctx, payment)
		return err

	case EventSubscriptionCanceled:
		return s.repo.RevokeBySource(ctx, providerName+":"+event.ProviderRef)

	default:
		return ErrUnhandledEvent
	}
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	return s.requireAdmin(ctx, callerID)
}

func (s *Service) requireAdmin(ctx context.Context, callerID string) error {
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
