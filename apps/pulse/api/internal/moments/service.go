package moments

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

// Save is "Save this moment ♥" (spec §31) - the caller must be a real
// participant in the underlying interaction (enforced by
// interactions.Get itself) and it must have actually completed; nothing
// beyond timestamp/participants/type/duration-or-pattern is ever
// stored, matching spec's own "without storing unnecessary content."
func (s *Service) Save(ctx context.Context, interactions PulseInteractions, callerID, interactionID string) (Moment, error) {
	ref, err := interactions.Get(ctx, callerID, interactionID)
	if err != nil {
		return Moment{}, err
	}
	if ref.Status != "completed" {
		return Moment{}, ErrNotCompleted
	}

	other := ref.SenderID
	if ref.SenderID == callerID {
		other = ref.ReceiverID
	}

	m := Moment{
		ID: uuid.NewString(), InteractionID: ref.ID, SavedByUserID: callerID, OtherUserID: other,
		InteractionType: ref.Type, DurationMs: ref.DurationMs, Pattern: ref.Pattern,
		OccurredAt: ref.CreatedAt, SavedAt: time.Now().UTC(),
	}
	return s.repo.Save(ctx, m)
}

func (s *Service) ListMine(ctx context.Context, callerID string, limit int) ([]Moment, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListMine(ctx, callerID, limit)
}

func (s *Service) Delete(ctx context.Context, callerID, id string) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if m.SavedByUserID != callerID {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
