package remoteconfig

import "context"

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service, no adapter needed. Writing and reading change history
// are platform.admin only: this is operational configuration, not
// per-user data.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	repo  Repository
	admin AdminChecker
}

func NewService(repo Repository, admin AdminChecker) *Service {
	return &Service{repo: repo, admin: admin}
}

func (s *Service) Set(ctx context.Context, callerID string, in SetInput) (Entry, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Entry{}, err
	}
	if err := in.Validate(); err != nil {
		return Entry{}, err
	}
	return s.repo.Set(ctx, callerID, in)
}

// Get and List are open to any authenticated caller - a running service
// or client needs to read its own config at runtime, and config values
// aren't sensitive per-user data.
func (s *Service) Get(ctx context.Context, appID, environment, key string) (Entry, error) {
	return s.repo.Get(ctx, appID, environment, key)
}

func (s *Service) List(ctx context.Context, appID, environment string) ([]Entry, error) {
	return s.repo.List(ctx, appID, environment)
}

func (s *Service) Delete(ctx context.Context, callerID, appID, environment, key, reason string) error {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, callerID, appID, environment, key, reason)
}

// History is platform.admin only, unlike Get/List - an audit trail of
// who changed what, when is a more sensitive, operationally-scoped
// concern than the current value itself.
func (s *Service) History(ctx context.Context, callerID, appID, environment, key string) ([]Change, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return nil, err
	}
	return s.repo.History(ctx, appID, environment, key)
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
