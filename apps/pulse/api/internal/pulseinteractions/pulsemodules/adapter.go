// Package pulsemodules holds pulse-interactions' adapter onto signals,
// a sibling Pulse module living in the same pulse-api binary - real,
// already-constructed *signals.Service value wired at the composition
// root (internal/api/router.go), never a duplicated copy of its data.
// Mirrors internal/mood/pulsemodules and internal/livetouch/pulsemodules'
// own precedent and naming - pulse-interactions' first dependency on a
// sibling Pulse module (every prior Type only ever needed Core
// capabilities).
package pulsemodules

import (
	"context"
	"errors"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	"github.com/example/core-platform/apps/pulse/api/internal/signals"
)

// SignalsAdapter adapts signals' real *Service.Get directly onto
// pulseinteractions.Signals - no per-caller Core client needed, since
// Get reads only Pulse's own Postgres and enforces ownership itself.
type SignalsAdapter struct {
	svc *signals.Service
}

func NewSignalsAdapter(svc *signals.Service) SignalsAdapter {
	return SignalsAdapter{svc: svc}
}

func (a SignalsAdapter) Get(ctx context.Context, callerID, signalID string) (pulseinteractions.SignalRef, error) {
	sig, err := a.svc.Get(ctx, callerID, signalID)
	if err != nil {
		// Translated to pulse-interactions' own sentinels so its
		// writeDomainError maps them to the right HTTP status (404/403)
		// without needing to know signals' own error values.
		switch {
		case errors.Is(err, signals.ErrNotFound):
			return pulseinteractions.SignalRef{}, pulseinteractions.ErrNotFound
		case errors.Is(err, signals.ErrForbidden):
			return pulseinteractions.SignalRef{}, pulseinteractions.ErrForbidden
		default:
			return pulseinteractions.SignalRef{}, err
		}
	}
	segs := make([]pulseinteractions.Segment, 0, len(sig.Segments))
	for _, s := range sig.Segments {
		segs = append(segs, pulseinteractions.Segment{Type: string(s.Type), DurationMs: s.DurationMs})
	}
	return pulseinteractions.SignalRef{ID: sig.ID, TargetUserID: sig.TargetUserID, Segments: segs}, nil
}
