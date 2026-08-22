// Package pulsemodules holds live-touch's adapter onto bond, a sibling
// Pulse module living in the same pulse-api binary - real,
// already-constructed *bond.Service value wired at the composition root
// (internal/api/router.go), never a duplicated copy of its data.
// Mirrors internal/mood/pulsemodules' own precedent and naming.
package pulsemodules

import (
	"context"
	"errors"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
)

// BondAdapter adapts bond's real *Service.MyActiveBond directly onto
// livetouch.Bond - no per-caller CoreRelationships needed, since
// MyActiveBond reads only Pulse's own Postgres.
type BondAdapter struct {
	svc *bond.Service
}

func NewBondAdapter(svc *bond.Service) BondAdapter {
	return BondAdapter{svc: svc}
}

func (a BondAdapter) MyActiveBond(ctx context.Context, callerID string) (livetouch.BondRef, error) {
	b, err := a.svc.MyActiveBond(ctx, callerID)
	if err != nil {
		if errors.Is(err, bond.ErrNotFound) {
			return livetouch.BondRef{}, livetouch.ErrNoBond
		}
		return livetouch.BondRef{}, err
	}
	return livetouch.BondRef{UserAID: b.UserAID, UserBID: b.UserBID}, nil
}
