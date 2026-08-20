// Package memory is an in-memory bond.Repository for tests. It
// reproduces the one-active-bond invariant with a mutex-guarded holder
// map - correct enough to prove the service-layer logic, but a Go
// mutex trivially serializes every call, so it cannot exercise the real
// concurrency risk the way postgres.Repository's transactional
// bond_active_holders table does; that's proven separately by live
// validation against real Postgres (see apps/pulse/docs/
// ARCHITECTURE_AUDIT.md's Risk #2 and this phase's VALIDATION-style
// notes).
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
)

type Repository struct {
	mu            sync.Mutex
	byID          map[string]bond.Bond
	activeHolders map[string]string // userID -> bondID, at most one entry per user, ever
}

func New() *Repository {
	return &Repository{byID: map[string]bond.Bond{}, activeHolders: map[string]string{}}
}

func (r *Repository) Create(ctx context.Context, b bond.Bond) (bond.Bond, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[b.ID] = b
	return b, nil
}

func (r *Repository) Get(ctx context.Context, id string) (bond.Bond, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return bond.Bond{}, bond.ErrNotFound
	}
	return b, nil
}

func (r *Repository) Accept(ctx context.Context, id string) (bond.Bond, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return bond.Bond{}, bond.ErrNotFound
	}
	if _, taken := r.activeHolders[b.UserAID]; taken {
		return bond.Bond{}, bond.ErrAlreadyBonded
	}
	if _, taken := r.activeHolders[b.UserBID]; taken {
		return bond.Bond{}, bond.ErrAlreadyBonded
	}
	now := time.Now().UTC()
	b.Status = bond.StatusActive
	b.AcceptedAt = &now
	b.UpdatedAt = now
	r.byID[id] = b
	r.activeHolders[b.UserAID] = id
	r.activeHolders[b.UserBID] = id
	return b, nil
}

func (r *Repository) End(ctx context.Context, id string) (bond.Bond, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return bond.Bond{}, bond.ErrNotFound
	}
	now := time.Now().UTC()
	b.Status = bond.StatusEnded
	b.EndedAt = &now
	b.UpdatedAt = now
	r.byID[id] = b
	if r.activeHolders[b.UserAID] == id {
		delete(r.activeHolders, b.UserAID)
	}
	if r.activeHolders[b.UserBID] == id {
		delete(r.activeHolders, b.UserBID)
	}
	return b, nil
}

func (r *Repository) ActiveForUser(ctx context.Context, userID string) (bond.Bond, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.activeHolders[userID]
	if !ok {
		return bond.Bond{}, bond.ErrNotFound
	}
	return r.byID[id], nil
}
