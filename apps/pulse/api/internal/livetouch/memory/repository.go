// Package memory is an in-memory livetouch.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]livetouch.Session
}

func New() *Repository {
	return &Repository{byID: map[string]livetouch.Session{}}
}

func (r *Repository) Create(ctx context.Context, s livetouch.Session) (livetouch.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.ID] = s
	return s, nil
}

func (r *Repository) Get(ctx context.Context, id string) (livetouch.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	return s, nil
}

func (r *Repository) Accept(ctx context.Context, id string, acceptedAt time.Time) (livetouch.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	s.Status = livetouch.StatusActive
	s.AcceptedAt = &acceptedAt
	s.UpdatedAt = acceptedAt
	r.byID[id] = s
	return s, nil
}

func (r *Repository) End(ctx context.Context, id string, endedAt time.Time, reason livetouch.EndReason) (livetouch.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	s.Status = livetouch.StatusEnded
	s.EndReason = &reason
	s.EndedAt = &endedAt
	if s.AcceptedAt != nil {
		durationMs := int(endedAt.Sub(*s.AcceptedAt).Milliseconds())
		s.DurationMs = &durationMs
	}
	s.UpdatedAt = endedAt
	r.byID[id] = s
	return s, nil
}
