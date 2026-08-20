// Package pulsemodules holds mood's adapters onto other Pulse modules
// living in the same pulse-api binary - pulse-connections' real
// Friend/Close-Friend classification and bond's real active-partner
// record. Both are wired at the composition root
// (internal/api/router.go) around the sibling module's real,
// already-constructed *Service value - never an HTTP self-call to
// Pulse's own API, and never a duplicated copy of their data. Named
// distinctly from the "core" package convention every other module
// uses for its Core Platform adapters, since these wrap sibling Pulse
// modules, not Core - mirrors this codebase's own precedent for
// same-process cross-module dependencies (see backend/core-api/internal
// /notifications/service.go's AdminChecker, "satisfied directly by
// *authz.Service - no adapter needed").
package pulsemodules

import (
	"context"
	"errors"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	"github.com/example/core-platform/apps/pulse/api/internal/mood"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
)

// ConnectionsAdapter adapts pulse-connections' real *Service - plus the
// caller's own CoreRelationships, built the exact same way pulse-
// connections' own http.go builds it - onto mood.Connections.
type ConnectionsAdapter struct {
	svc  *pulseconnections.Service
	core pulseconnections.CoreRelationships
}

func NewConnectionsAdapter(svc *pulseconnections.Service, core pulseconnections.CoreRelationships) ConnectionsAdapter {
	return ConnectionsAdapter{svc: svc, core: core}
}

func (a ConnectionsAdapter) ListMine(ctx context.Context, callerID string) ([]mood.ConnectionRef, error) {
	conns, err := a.svc.ListMine(ctx, a.core, callerID)
	if err != nil {
		return nil, err
	}
	out := make([]mood.ConnectionRef, 0, len(conns))
	for _, c := range conns {
		out = append(out, mood.ConnectionRef{OtherUserID: c.OtherUserID, Status: c.Status, Classification: string(c.Classification)})
	}
	return out, nil
}

// BondAdapter adapts bond's real *Service.MyActiveBond directly onto
// mood.Bond - no per-caller CoreRelationships needed, since
// MyActiveBond reads only Pulse's own Postgres (bond_active_holders),
// never Core.
type BondAdapter struct {
	svc *bond.Service
}

func NewBondAdapter(svc *bond.Service) BondAdapter {
	return BondAdapter{svc: svc}
}

func (a BondAdapter) MyActiveBond(ctx context.Context, callerID string) (mood.BondRef, error) {
	b, err := a.svc.MyActiveBond(ctx, callerID)
	if err != nil {
		if errors.Is(err, bond.ErrNotFound) {
			return mood.BondRef{}, mood.ErrNoBond
		}
		return mood.BondRef{}, err
	}
	return mood.BondRef{UserAID: b.UserAID, UserBID: b.UserBID}, nil
}
