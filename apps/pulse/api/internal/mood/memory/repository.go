// Package memory is an in-memory mood.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/mood"
)

type Repository struct {
	mu     sync.Mutex
	byUser map[string]mood.Mood
}

func New() *Repository {
	return &Repository{byUser: map[string]mood.Mood{}}
}

func (r *Repository) Set(ctx context.Context, m mood.Mood) (mood.Mood, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byUser[m.UserID] = m
	return m, nil
}

func (r *Repository) Get(ctx context.Context, userID string) (mood.Mood, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byUser[userID]
	if !ok || time.Now().UTC().After(m.ExpiresAt) {
		return mood.Mood{}, mood.ErrNotFound
	}
	return m, nil
}

func (r *Repository) Clear(ctx context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byUser, userID)
	return nil
}
