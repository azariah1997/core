package signals

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

// checkConnected mirrors pulse-interactions' own helper of the same
// name and meaning: an active Friend or Bond connection, checked from
// the caller's own real relationships.
func checkConnected(ctx context.Context, core CoreRelationships, otherUserID string) error {
	for _, relType := range []string{FriendRelationshipType, BondRelationshipType} {
		rels, err := core.ListMine(ctx, relType)
		if err != nil {
			return err
		}
		for _, r := range rels {
			if r.RequesterID != otherUserID && r.TargetID != otherUserID {
				continue
			}
			switch r.Status {
			case "active":
				return nil
			case "blocked":
				return ErrBlocked
			}
		}
	}
	return ErrNotConnected
}

// Create saves a new pattern bound to one specific real connection -
// checked at creation time the same way every other interaction in this
// codebase checks it, so a signal can never be created against a
// stranger or a blocked relationship.
func (s *Service) Create(ctx context.Context, core CoreRelationships, callerID string, in CreateInput) (Signal, error) {
	if err := in.Validate(); err != nil {
		return Signal{}, err
	}
	if err := checkConnected(ctx, core, in.TargetUserID); err != nil {
		return Signal{}, err
	}
	now := time.Now().UTC()
	sig := Signal{
		ID: uuid.NewString(), OwnerUserID: callerID, TargetUserID: in.TargetUserID,
		Label: in.Label, Segments: in.Segments, CreatedAt: now,
	}
	return s.repo.Create(ctx, sig)
}

func (s *Service) ListMine(ctx context.Context, callerID string) ([]Signal, error) {
	return s.repo.ListMine(ctx, callerID)
}

// Get enforces ownership - only the owner may ever read (and, via
// pulse-interactions' SendSignal, send) their own private pattern.
// This is the one method pulse-interactions calls directly (same
// same-process "satisfied directly" pattern mood/live-touch already
// established for bond), so this ownership check is Custom Signals'
// entire authorization surface for sending, not duplicated elsewhere.
func (s *Service) Get(ctx context.Context, callerID, id string) (Signal, error) {
	sig, err := s.repo.Get(ctx, id)
	if err != nil {
		return Signal{}, err
	}
	if sig.OwnerUserID != callerID {
		return Signal{}, ErrForbidden
	}
	return sig, nil
}

func (s *Service) Delete(ctx context.Context, callerID, id string) error {
	sig, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if sig.OwnerUserID != callerID {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
