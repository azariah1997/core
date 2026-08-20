// Package memory is an in-memory pulseinteractions.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
)

type Repository struct {
	mu           sync.Mutex
	byID         map[string]pulseinteractions.Interaction
	byIdempotent map[string]string // senderID|clientRequestID -> interaction ID
}

func New() *Repository {
	return &Repository{byID: map[string]pulseinteractions.Interaction{}, byIdempotent: map[string]string{}}
}

func idempotentKey(senderID, clientRequestID string) string { return senderID + "|" + clientRequestID }

func (r *Repository) Create(ctx context.Context, i pulseinteractions.Interaction, clientRequestID string) (pulseinteractions.Interaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if clientRequestID != "" {
		key := idempotentKey(i.SenderID, clientRequestID)
		if existingID, ok := r.byIdempotent[key]; ok {
			return r.byID[existingID], nil
		}
		r.byIdempotent[key] = i.ID
	}
	r.byID[i.ID] = i
	return i, nil
}

func (r *Repository) Get(ctx context.Context, id string) (pulseinteractions.Interaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.byID[id]
	if !ok {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	return i, nil
}

func (r *Repository) Start(ctx context.Context, id string, startedAt time.Time) (pulseinteractions.Interaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.byID[id]
	if !ok {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	i.Status = pulseinteractions.StatusStarted
	i.StartedAt = &startedAt
	i.UpdatedAt = startedAt
	r.byID[id] = i
	return i, nil
}

func (r *Repository) Stop(ctx context.Context, id string, endedAt time.Time) (pulseinteractions.Interaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.byID[id]
	if !ok {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	i.Status = pulseinteractions.StatusCompleted
	i.EndedAt = &endedAt
	if i.StartedAt != nil {
		durationMs := int(endedAt.Sub(*i.StartedAt).Milliseconds())
		i.DurationMs = &durationMs
	}
	i.UpdatedAt = endedAt
	r.byID[id] = i
	return i, nil
}

func (r *Repository) ListForUser(ctx context.Context, userID string, limit int) ([]pulseinteractions.Interaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]pulseinteractions.Interaction, 0, len(r.byID))
	for _, i := range r.byID {
		if i.SenderID == userID || i.ReceiverID == userID {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
