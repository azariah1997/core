// Package memory is an in-memory pulseconnections.ClassificationRepository
// for tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
)

type Repository struct {
	mu    sync.Mutex
	byKey map[string]pulseconnections.Classification // relationshipID|ownerUserID -> classification
}

func New() *Repository {
	return &Repository{byKey: map[string]pulseconnections.Classification{}}
}

func key(relationshipID, ownerUserID string) string { return relationshipID + "|" + ownerUserID }

func (r *Repository) Set(ctx context.Context, relationshipID, ownerUserID string, c pulseconnections.Classification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[key(relationshipID, ownerUserID)] = c
	return nil
}

func (r *Repository) Get(ctx context.Context, relationshipID, ownerUserID string) (pulseconnections.Classification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.byKey[key(relationshipID, ownerUserID)]; ok {
		return c, nil
	}
	return pulseconnections.ClassificationFriend, nil
}

func (r *Repository) ListForOwner(ctx context.Context, ownerUserID string) (map[string]pulseconnections.Classification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]pulseconnections.Classification{}
	suffix := "|" + ownerUserID
	for k, c := range r.byKey {
		if len(k) > len(suffix) && k[len(k)-len(suffix):] == suffix {
			out[k[:len(k)-len(suffix)]] = c
		}
	}
	return out, nil
}
