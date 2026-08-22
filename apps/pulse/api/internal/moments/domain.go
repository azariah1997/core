// Package moments is Pulse's private saved-highlights timeline (product
// spec §30-31, Phase 12): "Pulse may maintain a private timeline of
// meaningful interactions... avoid turning this into chat history."
// A Moment is a personal bookmark - each participant may independently
// choose to save the same underlying interaction to their own timeline
// (saving is never automatically shared with the other participant,
// matching every consumer product's own "save" semantics) - and it
// stores exactly what spec §31 names (timestamp, participants,
// interaction type, duration/pattern) plus a reference to the real
// interaction, never a duplicated copy of any richer payload (no
// message content, ever - "no chat" is this module's one hard rule).
package moments

import (
	"context"
	"errors"
	"time"
)

// Moment is one saved highlight. OccurredAt is the underlying
// interaction's own CreatedAt (when it actually happened) - distinct
// from SavedAt (when this participant chose to remember it), which
// could be much later if they save something old.
type Moment struct {
	ID              string
	InteractionID   string
	SavedByUserID   string
	OtherUserID     string
	InteractionType string
	DurationMs      *int
	Pattern         *string
	OccurredAt      time.Time
	SavedAt         time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("moment not found")
	ErrForbidden = errors.New("not permitted to perform this action on this moment")
	// ErrNotCompleted is Save's own answer to "this interaction hasn't
	// actually happened yet" - only a completed interaction has a real
	// duration/pattern worth remembering; a merely created or
	// in-progress one has nothing to save.
	ErrNotCompleted = errors.New("only a completed interaction can be saved as a moment")
)

// InteractionRef is pulse-interactions' own Interaction, narrowed to
// what a Moment needs to display - duplicated rather than imported
// (this codebase's consumer-defined-interface convention).
type InteractionRef struct {
	ID         string
	Type       string
	SenderID   string
	ReceiverID string
	Status     string
	DurationMs *int
	Pattern    *string
	CreatedAt  time.Time
}

// PulseInteractions resolves one real interaction by ID - satisfied
// directly by pulse-interactions' real *Service.Get (same "satisfied
// directly by a real same-process *Service" pattern mood/live-touch/
// pulse-interactions itself already established for bond/signals).
// Get already enforces that only the interaction's real sender or
// receiver may read it - Save never re-checks participation itself,
// the exact same reuse signals.Service.Get's ownership check gets for
// pulse-interactions' own SendSignal.
type PulseInteractions interface {
	Get(ctx context.Context, callerID, interactionID string) (InteractionRef, error)
}

// Repository is Pulse's own moment storage. Save is idempotent on
// (savedByUserID, interactionID) - saving the same interaction twice
// returns the original row, matching this codebase's own idempotent-
// creation convention (e.g. pulse-interactions' own client_request_id).
type Repository interface {
	Save(ctx context.Context, m Moment) (Moment, error)
	Get(ctx context.Context, id string) (Moment, error)
	ListMine(ctx context.Context, userID string, limit int) ([]Moment, error)
	Delete(ctx context.Context, id string) error
}
