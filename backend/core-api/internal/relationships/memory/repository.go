// Package memory is an in-memory relationships.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/relationships"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]relationships.Relationship
}

func New() *Repository {
	return &Repository{byID: map[string]relationships.Relationship{}}
}

func pairKey(appID, a, b, relType string) string {
	if a > b {
		a, b = b, a
	}
	return appID + "|" + a + "|" + b + "|" + relType
}

func (r *Repository) Create(ctx context.Context, in relationships.RequestInput, status relationships.Status) (relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := pairKey(in.AppID, in.RequesterID, in.TargetID, in.Type)
	for _, rel := range r.byID {
		if pairKey(rel.AppID, rel.RequesterID, rel.TargetID, rel.Type) == key {
			return relationships.Relationship{}, relationships.ErrExists
		}
	}

	now := time.Now().UTC()
	rel := relationships.Relationship{
		ID: uuid.NewString(), AppID: in.AppID, RequesterID: in.RequesterID, TargetID: in.TargetID,
		Type: in.Type, Status: status, Metadata: in.Metadata, CreatedAt: now, UpdatedAt: now,
	}
	r.byID[rel.ID] = rel
	return rel, nil
}

func (r *Repository) Get(ctx context.Context, id string) (relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.byID[id]
	if !ok {
		return relationships.Relationship{}, relationships.ErrNotFound
	}
	return rel, nil
}

func (r *Repository) FindBetween(ctx context.Context, appID, userA, userB, relType string) (relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := pairKey(appID, userA, userB, relType)
	for _, rel := range r.byID {
		if pairKey(rel.AppID, rel.RequesterID, rel.TargetID, rel.Type) == key {
			return rel, nil
		}
	}
	return relationships.Relationship{}, relationships.ErrNotFound
}

func (r *Repository) ListForUser(ctx context.Context, appID, userID string, filter relationships.ListFilter) ([]relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []relationships.Relationship
	for _, rel := range r.byID {
		if rel.AppID != appID {
			continue
		}
		if rel.RequesterID != userID && rel.TargetID != userID {
			continue
		}
		if filter.Type != "" && rel.Type != filter.Type {
			continue
		}
		if filter.Status != "" && rel.Status != filter.Status {
			continue
		}
		list = append(list, rel)
	}
	return list, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status relationships.Status) (relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.byID[id]
	if !ok {
		return relationships.Relationship{}, relationships.ErrNotFound
	}
	rel.Status = status
	rel.UpdatedAt = time.Now().UTC()
	r.byID[id] = rel
	return rel, nil
}

func (r *Repository) Revive(ctx context.Context, id string, in relationships.RequestInput) (relationships.Relationship, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.byID[id]
	if !ok {
		return relationships.Relationship{}, relationships.ErrNotFound
	}
	rel.RequesterID = in.RequesterID
	rel.TargetID = in.TargetID
	rel.Status = relationships.StatusPending
	rel.Metadata = in.Metadata
	rel.UpdatedAt = time.Now().UTC()
	r.byID[id] = rel
	return rel, nil
}
