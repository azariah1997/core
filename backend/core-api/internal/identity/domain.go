// Package identity abstracts "who authenticated" from any specific
// identity provider. Platform code depends on Provider and Repository,
// never on Keycloak (or any other IdP) directly, so a future provider
// (Google, Apple, Microsoft, passkeys) is an additional implementation,
// not a rewrite. Identity is deliberately separate from User (Phase 4):
// Identity is "who authenticated", User is "the platform person/account".
package identity

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Provider is the external IdP boundary. Not every provider can meaningfully
// implement every method - a federated provider (Google, Apple) can
// ValidateToken and GetIdentity but has no CreateIdentity/DisableIdentity of
// its own; such implementations return ErrUnsupportedOperation.
type Provider interface {
	ValidateToken(ctx context.Context, token string) (Claims, error)
	CreateIdentity(ctx context.Context, in CreateIdentityInput) (ProviderIdentity, error)
	DisableIdentity(ctx context.Context, providerSubject string) error
	GetIdentity(ctx context.Context, providerSubject string) (ProviderIdentity, error)
}

// Claims is what a validated token tells us about the caller.
type Claims struct {
	Subject  string // the provider's stable subject identifier - never used as a platform primary key
	Username string
	Email    string
}

type CreateIdentityInput struct {
	Username string
	Email    string
	Password string
}

// ProviderIdentity is the provider's view of an identity, distinct from the
// platform's own Identity record.
type ProviderIdentity struct {
	ProviderSubject string
	Username        string
	Email           string
	Enabled         bool
}

var ErrUnsupportedOperation = errors.New("operation not supported by this identity provider")

// Identity is the platform's own linkage record: which (provider,
// providerSubject) pairs have authenticated, and which platform User (once
// Phase 4 exists) they're linked to. UserID is nullable: an identity can
// authenticate before it's provisioned as a platform User.
type Identity struct {
	ID              string
	UserID          *string
	Provider        string
	ProviderSubject string
	Status          Status
	CreatedAt       time.Time
	LastLoginAt     *time.Time
}

var ErrNotFound = errors.New("identity not found")

// Repository is the storage-agnostic boundary for platform Identity
// linkage records - not to be confused with Provider, which talks to the
// external IdP.
type Repository interface {
	// GetByProviderSubject returns ErrNotFound if no linkage exists yet.
	GetByProviderSubject(ctx context.Context, provider, providerSubject string) (Identity, error)
	// Touch records a login: creating the linkage record on first sight,
	// or updating LastLoginAt on subsequent ones. Idempotent per call.
	Touch(ctx context.Context, provider, providerSubject string) (Identity, error)
	Disable(ctx context.Context, id string) error
}
