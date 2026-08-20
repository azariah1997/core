package bond

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// hasActiveFriendConnection checks product spec §11's required
// precondition ("existing connection -> Request Bond") by listing the
// caller's real pulse_friend relationships from Core - never a second,
// Pulse-duplicated connection table.
func hasActiveFriendConnection(ctx context.Context, core CoreRelationships, callerID, targetUserID string) (bool, error) {
	friends, err := core.ListMine(ctx, FriendRelationshipType)
	if err != nil {
		return false, err
	}
	for _, f := range friends {
		if f.Status != "active" {
			continue
		}
		if f.RequesterID == targetUserID || f.TargetID == targetUserID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) RequestBond(ctx context.Context, core CoreRelationships, callerID string, in RequestInput) (Bond, error) {
	if err := in.Validate(); err != nil {
		return Bond{}, err
	}
	connected, err := hasActiveFriendConnection(ctx, core, callerID, in.TargetUserID)
	if err != nil {
		return Bond{}, err
	}
	if !connected {
		return Bond{}, ErrNoConnection
	}
	rel, err := core.Request(ctx, in.TargetUserID, RelationshipType)
	if err != nil {
		return Bond{}, err
	}
	now := time.Now().UTC()
	b := Bond{
		ID: uuid.NewString(), RelationshipID: rel.ID, UserAID: callerID, UserBID: in.TargetUserID,
		Status: StatusPending, RequestedAt: now, UpdatedAt: now,
	}
	return s.repo.Create(ctx, b)
}

// Accept may only be called by UserBID (the target) - mirroring Core's
// own relationships.Accept semantics exactly (only the target may
// accept/decline a pending request).
func (s *Service) Accept(ctx context.Context, core CoreRelationships, callerID, bondID string) (Bond, error) {
	b, err := s.repo.Get(ctx, bondID)
	if err != nil {
		return Bond{}, err
	}
	if b.UserBID != callerID {
		return Bond{}, ErrForbidden
	}
	if _, err := core.Accept(ctx, b.RelationshipID); err != nil {
		return Bond{}, err
	}
	// The real one-active-bond enforcement happens inside this call -
	// see Repository's doc comment. If it returns ErrAlreadyBonded,
	// Core's own relationship was already flipped to active above; that
	// mismatch (Core says active, Pulse says still pending) is an
	// accepted, narrow inconsistency window until the caller retries
	// End/Decline to reconcile - documented, not silently papered over.
	return s.repo.Accept(ctx, bondID)
}

func (s *Service) Decline(ctx context.Context, core CoreRelationships, callerID, bondID string) (Bond, error) {
	b, err := s.repo.Get(ctx, bondID)
	if err != nil {
		return Bond{}, err
	}
	if b.UserBID != callerID {
		return Bond{}, ErrForbidden
	}
	if _, err := core.Decline(ctx, b.RelationshipID); err != nil {
		return Bond{}, err
	}
	return s.repo.End(ctx, bondID)
}

func (s *Service) End(ctx context.Context, core CoreRelationships, callerID, bondID string) (Bond, error) {
	b, err := s.repo.Get(ctx, bondID)
	if err != nil {
		return Bond{}, err
	}
	if b.UserAID != callerID && b.UserBID != callerID {
		return Bond{}, ErrForbidden
	}
	if err := core.Remove(ctx, b.RelationshipID); err != nil {
		return Bond{}, err
	}
	return s.repo.End(ctx, bondID)
}

func (s *Service) MyActiveBond(ctx context.Context, callerID string) (Bond, error) {
	return s.repo.ActiveForUser(ctx, callerID)
}
