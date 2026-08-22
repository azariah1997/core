// Package memory is an in-memory signals.Repository for tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/core-platform/apps/pulse/api/internal/signals"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]signals.Signal
}

func New() *Repository {
	return &Repository{byID: map[string]signals.Signal{}}
}

func (r *Repository) Create(ctx context.Context, s signals.Signal) (signals.Signal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.ID] = s
	return s, nil
}

func (r *Repository) Get(ctx context.Context, id string) (signals.Signal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return signals.Signal{}, signals.ErrNotFound
	}
	return s, nil
}

func (r *Repository) ListMine(ctx context.Context, ownerUserID string) ([]signals.Signal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []signals.Signal
	for _, s := range r.byID {
		if s.OwnerUserID == ownerUserID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}
