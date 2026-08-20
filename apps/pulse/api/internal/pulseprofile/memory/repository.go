// Package memory is an in-memory pulseprofile.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
)

type Repository struct {
	mu       sync.Mutex
	byUserID map[string]pulseprofile.Profile
	byHandle map[string]string // handle -> userID
}

func New() *Repository {
	return &Repository{
		byUserID: map[string]pulseprofile.Profile{},
		byHandle: map[string]string{},
	}
}

func (r *Repository) Create(ctx context.Context, p pulseprofile.Profile) (pulseprofile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byHandle[p.Handle]; exists {
		return pulseprofile.Profile{}, pulseprofile.ErrHandleTaken
	}
	r.byUserID[p.UserID] = p
	r.byHandle[p.Handle] = p.UserID
	return p, nil
}

func (r *Repository) Get(ctx context.Context, userID string) (pulseprofile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUserID[userID]
	if !ok {
		return pulseprofile.Profile{}, pulseprofile.ErrNotFound
	}
	return p, nil
}

func (r *Repository) GetByHandle(ctx context.Context, handle string) (pulseprofile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	userID, ok := r.byHandle[handle]
	if !ok {
		return pulseprofile.Profile{}, pulseprofile.ErrNotFound
	}
	return r.byUserID[userID], nil
}

func (r *Repository) Update(ctx context.Context, userID string, in pulseprofile.UpdateInput) (pulseprofile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUserID[userID]
	if !ok {
		return pulseprofile.Profile{}, pulseprofile.ErrNotFound
	}
	if in.VisualPrefs != nil {
		p.VisualPrefs = in.VisualPrefs
	}
	if in.PulsePrefs != nil {
		p.PulsePrefs = in.PulsePrefs
	}
	p.UpdatedAt = time.Now().UTC()
	r.byUserID[userID] = p
	return p, nil
}
