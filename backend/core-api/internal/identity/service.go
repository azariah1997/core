package identity

import "context"

// Service is the entry point HTTP middleware (and, later, any gRPC/SDK
// surface) depends on. It never talks to Postgres or Keycloak directly.
type Service struct {
	providerName string
	provider     Provider
	repo         Repository
}

func NewService(providerName string, provider Provider, repo Repository) *Service {
	return &Service{providerName: providerName, provider: provider, repo: repo}
}

// Authenticate validates a bearer token and records the login, creating the
// platform-side linkage record on first sight. It is the single place
// "validate a token" and "remember that this identity authenticated" happen
// together, so callers can't do one without the other.
//
// It returns the raw Claims alongside Identity because the linkage record
// deliberately doesn't persist profile data (username/email) - callers that
// need it for one-time decisions (like what to name a newly provisioned
// User) get it from the token itself, not from a second store.
func (s *Service) Authenticate(ctx context.Context, token string) (Identity, Claims, error) {
	claims, err := s.provider.ValidateToken(ctx, token)
	if err != nil {
		return Identity{}, Claims{}, err
	}
	id, err := s.repo.Touch(ctx, s.providerName, claims.Subject)
	if err != nil {
		return Identity{}, Claims{}, err
	}
	return id, claims, nil
}

func (s *Service) Disable(ctx context.Context, id string) error {
	return s.repo.Disable(ctx, id)
}

// LinkUser records which platform User this identity was provisioned as.
func (s *Service) LinkUser(ctx context.Context, identityID, userID string) error {
	return s.repo.LinkUser(ctx, identityID, userID)
}
