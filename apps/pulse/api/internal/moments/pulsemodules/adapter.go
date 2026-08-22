// Package pulsemodules holds moments' adapter onto pulse-interactions,
// a sibling Pulse module living in the same pulse-api binary - real,
// already-constructed *pulseinteractions.Service value wired at the
// composition root (internal/api/router.go), never a duplicated copy
// of its data. Mirrors mood/live-touch/pulse-interactions' own
// pulsemodules precedent and naming.
package pulsemodules

import (
	"context"
	"errors"

	"github.com/example/core-platform/apps/pulse/api/internal/moments"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
)

// InteractionsAdapter adapts pulse-interactions' real *Service.Get
// directly onto moments.PulseInteractions - no per-caller Core client
// needed, since Get reads only Pulse's own Postgres and enforces
// participation itself.
type InteractionsAdapter struct {
	svc *pulseinteractions.Service
}

func NewInteractionsAdapter(svc *pulseinteractions.Service) InteractionsAdapter {
	return InteractionsAdapter{svc: svc}
}

func (a InteractionsAdapter) Get(ctx context.Context, callerID, interactionID string) (moments.InteractionRef, error) {
	i, err := a.svc.Get(ctx, callerID, interactionID)
	if err != nil {
		// Translated to moments' own sentinels so its writeDomainError
		// maps them to the right HTTP status without needing to know
		// pulse-interactions' own error values - the same translation
		// pulse-interactions' own SignalsAdapter already does for
		// signals' errors.
		switch {
		case errors.Is(err, pulseinteractions.ErrNotFound):
			return moments.InteractionRef{}, moments.ErrNotFound
		case errors.Is(err, pulseinteractions.ErrForbidden):
			return moments.InteractionRef{}, moments.ErrForbidden
		default:
			return moments.InteractionRef{}, err
		}
	}
	ref := moments.InteractionRef{
		ID: i.ID, Type: string(i.Type), SenderID: i.SenderID, ReceiverID: i.ReceiverID,
		Status: string(i.Status), DurationMs: i.DurationMs, CreatedAt: i.CreatedAt,
	}
	// The raw Pattern string (Knock's small pattern name, or a Signal's
	// own segments JSON) is exactly what spec §31 means by "duration/
	// pattern" - structural metadata, not chat content, so it's fine
	// for a Moment to keep as-is.
	if i.Pattern != nil {
		p := *i.Pattern
		ref.Pattern = &p
	}
	return ref, nil
}
