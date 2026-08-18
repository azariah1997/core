package relationships

import (
	"context"
	"errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Request creates a pending relationship, or revives one back to pending
// if the only existing row for this pair+type has ended - anything still
// live (pending/active/blocked) returns ErrExists instead; the caller
// should Accept/Decline/Remove/Block the existing row.
func (s *Service) Request(ctx context.Context, in RequestInput) (Relationship, error) {
	if err := in.Validate(); err != nil {
		return Relationship{}, err
	}
	existing, err := s.repo.FindBetween(ctx, in.AppID, in.RequesterID, in.TargetID, in.Type)
	if err == nil {
		if existing.Status != StatusEnded {
			return Relationship{}, ErrExists
		}
		return s.repo.Revive(ctx, existing.ID, in)
	}
	if !errors.Is(err, ErrNotFound) {
		return Relationship{}, err
	}
	return s.repo.Create(ctx, in, StatusPending)
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Relationship, error) {
	rel, err := s.repo.Get(ctx, id)
	if err != nil {
		return Relationship{}, err
	}
	if rel.RequesterID != callerID && rel.TargetID != callerID {
		return Relationship{}, ErrForbidden
	}
	return rel, nil
}

func (s *Service) ListMine(ctx context.Context, appID, callerID string, filter ListFilter) ([]Relationship, error) {
	return s.repo.ListForUser(ctx, appID, callerID, filter)
}

// Accept and Decline may only be performed by the target - the recipient
// of the request, never the requester.
func (s *Service) Accept(ctx context.Context, callerID, id string) (Relationship, error) {
	rel, err := s.repo.Get(ctx, id)
	if err != nil {
		return Relationship{}, err
	}
	if rel.Status != StatusPending {
		return Relationship{}, ErrInvalidTransition
	}
	if rel.TargetID != callerID {
		return Relationship{}, ErrForbidden
	}
	return s.repo.UpdateStatus(ctx, id, StatusActive)
}

func (s *Service) Decline(ctx context.Context, callerID, id string) (Relationship, error) {
	rel, err := s.repo.Get(ctx, id)
	if err != nil {
		return Relationship{}, err
	}
	if rel.Status != StatusPending {
		return Relationship{}, ErrInvalidTransition
	}
	if rel.TargetID != callerID {
		return Relationship{}, ErrForbidden
	}
	return s.repo.UpdateStatus(ctx, id, StatusEnded)
}

// Remove cancels a still-pending request (requester only, "I take it
// back") or ends an active relationship (either participant, "unfriend").
func (s *Service) Remove(ctx context.Context, callerID, id string) (Relationship, error) {
	rel, err := s.repo.Get(ctx, id)
	if err != nil {
		return Relationship{}, err
	}
	switch rel.Status {
	case StatusPending:
		if rel.RequesterID != callerID {
			return Relationship{}, ErrForbidden
		}
	case StatusActive:
		if rel.RequesterID != callerID && rel.TargetID != callerID {
			return Relationship{}, ErrForbidden
		}
	default:
		return Relationship{}, ErrInvalidTransition
	}
	return s.repo.UpdateStatus(ctx, id, StatusEnded)
}

// Block works whether or not a relationship already exists - blocking a
// stranger creates one directly in the blocked state; blocking an
// existing pending/active/ended relationship transitions it. Either
// participant of an existing row may block through it (not just whoever
// requested it originally).
func (s *Service) Block(ctx context.Context, callerID, appID, targetID, relType string) (Relationship, error) {
	in := RequestInput{AppID: appID, RequesterID: callerID, TargetID: targetID, Type: relType}
	if err := in.Validate(); err != nil {
		return Relationship{}, err
	}

	existing, err := s.repo.FindBetween(ctx, appID, callerID, targetID, relType)
	if err == nil {
		if existing.RequesterID != callerID && existing.TargetID != callerID {
			return Relationship{}, ErrForbidden
		}
		return s.repo.UpdateStatus(ctx, existing.ID, StatusBlocked)
	}
	if !errors.Is(err, ErrNotFound) {
		return Relationship{}, err
	}
	return s.repo.Create(ctx, in, StatusBlocked)
}
