package pulseprofile

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// EnsureProfile returns the caller's existing profile, or creates one
// with the given handle if this is their first call - mirroring Core
// users.Service's own auto-provision-on-first-login pattern, so the
// mobile app never has to special-case "new user" as a separate flow.
func (s *Service) EnsureProfile(ctx context.Context, userID string, in CreateInput) (Profile, error) {
	existing, err := s.repo.Get(ctx, userID)
	if err == nil {
		return existing, nil
	}
	if err != ErrNotFound {
		return Profile{}, err
	}
	if err := in.Validate(); err != nil {
		return Profile{}, err
	}
	now := time.Now().UTC()
	p := Profile{
		UserID:    userID,
		Handle:    strings.ToLower(strings.TrimSpace(in.Handle)),
		CreatedAt: now,
		UpdatedAt: now,
	}
	// A handle collision surfaces as ErrHandleTaken from the repository,
	// not checked separately here - avoids a check-then-create race the
	// same way billing/authz's own repositories enforce uniqueness at
	// the storage layer, not in application code.
	return s.repo.Create(ctx, p)
}

func (s *Service) Get(ctx context.Context, userID string) (Profile, error) {
	return s.repo.Get(ctx, userID)
}

func (s *Service) GetByHandle(ctx context.Context, handle string) (Profile, error) {
	return s.repo.GetByHandle(ctx, strings.ToLower(strings.TrimSpace(handle)))
}

func (s *Service) Update(ctx context.Context, userID string, in UpdateInput) (Profile, error) {
	return s.repo.Update(ctx, userID, in)
}
