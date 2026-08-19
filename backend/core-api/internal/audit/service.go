package audit

import (
	"context"

	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service, no adapter needed. Reading audit records (Get/List) is
// platform.admin only: an audit trail routinely contains cross-user
// information (who did what to whom), a more sensitive read than most of
// this platform's other admin-gated data.
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

// Record is open to any authenticated caller (the HTTP handler always
// forces ActorUserID to the caller's own identity, never trusting the
// request body - see http.go) and to trusted in-process Go callers, like
// authz.Service reporting a role change, which may legitimately record
// an action on behalf of the acting admin rather than themselves.
// CorrelationID is always pulled from context, matching how every other
// module's outbox events already get theirs - never caller-supplied, so
// it can't be spoofed to make an unrelated write look connected to a
// given request.
func (s *Service) Record(ctx context.Context, in RecordInput) (Record, error) {
	if err := in.Validate(); err != nil {
		return Record{}, err
	}
	return s.repo.Record(ctx, in, correlation.FromContext(ctx))
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Record, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return Record{}, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, callerID string, filter ListFilter) (ListResult, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return ListResult{}, err
	}
	if filter.Limit <= 0 || filter.Limit > maxListLimit {
		filter.Limit = defaultListLimit
	}
	return s.repo.List(ctx, filter)
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
