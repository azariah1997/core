// Package memory is an in-memory pulseprefs.Repository for tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
)

type Repository struct {
	mu     sync.Mutex
	byUser map[string]pulseprefs.Preferences
}

func New() *Repository {
	return &Repository{byUser: map[string]pulseprefs.Preferences{}}
}

func (r *Repository) Get(ctx context.Context, userID string) (pulseprefs.Preferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUser[userID]
	if !ok {
		return pulseprefs.DefaultPreferences(userID), nil
	}
	return p, nil
}

func (r *Repository) Set(ctx context.Context, p pulseprefs.Preferences) (pulseprefs.Preferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byUser[p.UserID] = p
	return p, nil
}
