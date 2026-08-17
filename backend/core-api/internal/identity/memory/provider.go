package memory

import (
	"context"

	"github.com/example/core-platform/backend/core-api/internal/identity"
)

// Provider is a fake identity.Provider for tests: the token string itself
// is treated as the subject, with no signature verification. Never use
// outside tests.
type Provider struct{}

func (Provider) ValidateToken(ctx context.Context, token string) (identity.Claims, error) {
	if token == "" {
		return identity.Claims{}, identity.ErrNotFound
	}
	return identity.Claims{Subject: token, Username: token}, nil
}

func (Provider) CreateIdentity(ctx context.Context, in identity.CreateIdentityInput) (identity.ProviderIdentity, error) {
	return identity.ProviderIdentity{}, identity.ErrUnsupportedOperation
}

func (Provider) DisableIdentity(ctx context.Context, providerSubject string) error {
	return identity.ErrUnsupportedOperation
}

func (Provider) GetIdentity(ctx context.Context, providerSubject string) (identity.ProviderIdentity, error) {
	return identity.ProviderIdentity{}, identity.ErrUnsupportedOperation
}
