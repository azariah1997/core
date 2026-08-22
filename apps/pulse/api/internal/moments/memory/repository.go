// Package memory is an in-memory moments.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/example/core-platform/apps/pulse/api/internal/moments"
)

type Repository struct {
	mu           sync.Mutex
	byID         map[string]moments.Moment
	byIdempotent map[string]string // savedByUserID|interactionID -> moment ID
}

func New() *Repository {
	return &Repository{byID: map[string]moments.Moment{}, byIdempotent: map[string]string{}}
}

func idempotentKey(savedByUserID, interactionID string) string {
	return savedByUserID + "|" + interactionID
}

func (r *Repository) Save(ctx context.Context, m moments.Moment) (moments.Moment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := idempotentKey(m.SavedByUserID, m.InteractionID)
	if existingID, ok := r.byIdempotent[key]; ok {
		return r.byID[existingID], nil
	}
	r.byIdempotent[key] = m.ID
	r.byID[m.ID] = m
	return m, nil
}

func (r *Repository) Get(ctx context.Context, id string) (moments.Moment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return moments.Moment{}, moments.ErrNotFound
	}
	return m, nil
}

func (r *Repository) ListMine(ctx context.Context, userID string, limit int) ([]moments.Moment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]moments.Moment, 0, len(r.byID))
	for _, m := range r.byID {
		if m.SavedByUserID == userID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].SavedAt.After(out[b].SavedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return nil
}
