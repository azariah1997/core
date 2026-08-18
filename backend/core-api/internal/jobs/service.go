package jobs

import (
	"context"
	"time"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// AdminChecker mirrors search.AdminChecker/files.AdminChecker/
// notifications.AdminChecker - satisfied directly by *authz.Service, no
// adapter needed.
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

// Enqueue is the entire HTTP-facing cost of scheduling work: one insert.
// Execution happens later, in the worker process - this method never
// runs a handler, per the roadmap's "do not execute heavy jobs inside
// HTTP request handlers."
func (s *Service) Enqueue(ctx context.Context, callerID string, in EnqueueInput) (Job, error) {
	if err := in.Validate(); err != nil {
		return Job{}, err
	}
	now := time.Now()
	return s.repo.Create(ctx, callerID, in, in.resolveRunAt(now), in.resolveMaxAttempts(), in.resolveRecurrenceInterval())
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Job, error) {
	j, err := s.repo.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, j.CreatedBy); err != nil {
		return Job{}, err
	}
	return j, nil
}

func (s *Service) ListMine(ctx context.Context, callerID string, params ListParams) (ListResult, error) {
	if params.Limit <= 0 || params.Limit > maxListLimit {
		params.Limit = defaultListLimit
	}
	return s.repo.ListForCaller(ctx, callerID, params)
}

func (s *Service) ListAttempts(ctx context.Context, callerID, jobID string) ([]JobAttempt, error) {
	j, err := s.repo.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, j.CreatedBy); err != nil {
		return nil, err
	}
	return s.repo.ListAttempts(ctx, jobID)
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
