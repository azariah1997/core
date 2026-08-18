package search

import (
	"context"

	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// AdminChecker mirrors notifications.AdminChecker/files.AdminChecker -
// satisfied directly by *authz.Service, no adapter needed.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	provider searchidx.Provider
	admin    AdminChecker
}

func NewService(provider searchidx.Provider, admin AdminChecker) *Service {
	return &Service{provider: provider, admin: admin}
}

// Query is open to any authenticated caller - search only ever surfaces
// what's already been indexed, and results are scoped by the optional
// Type/AppID filters the caller supplies. Per-document visibility (e.g.
// "can this caller see this specific message") is deliberately not
// enforced here: that's the owning domain module's job, and doing it
// properly would mean this package re-implementing every other module's
// access rules. Documented, not silently skipped - see the package
// README.
func (s *Service) Query(ctx context.Context, in QueryInput) (searchidx.QueryResult, error) {
	if err := in.Validate(); err != nil {
		return searchidx.QueryResult{}, err
	}
	if in.Limit <= 0 || in.Limit > maxLimit {
		in.Limit = defaultLimit
	}
	return s.provider.Query(ctx, toParams(in))
}

// Index is the manual/on-demand re-indexing path, platform.admin only -
// the automatic path is worker's event-driven indexer, which talks to
// searchidx.Provider directly rather than through this HTTP-facing
// Service.
func (s *Service) Index(ctx context.Context, callerID string, in IndexInput) error {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return err
	}
	if err := in.Validate(); err != nil {
		return err
	}
	return s.provider.Index(ctx, toDocument(in))
}

func (s *Service) Delete(ctx context.Context, callerID, docType, appID, id string) error {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return err
	}
	return s.provider.Delete(ctx, docType, appID, id)
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
