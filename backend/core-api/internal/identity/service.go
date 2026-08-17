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
func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	claims, err := s.provider.ValidateToken(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	return s.repo.Touch(ctx, s.providerName, claims.Subject)
}

func (s *Service) Disable(ctx context.Context, id string) error {
	return s.repo.Disable(ctx, id)
}
