// Package memory is an in-memory identity.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/identity"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]identity.Identity
	// key is provider + "|" + providerSubject
	byLinkage map[string]string
}

func New() *Repository {
	return &Repository{byID: map[string]identity.Identity{}, byLinkage: map[string]string{}}
}

func key(provider, subject string) string { return provider + "|" + subject }

func (r *Repository) GetByProviderSubject(ctx context.Context, provider, providerSubject string) (identity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byLinkage[key(provider, providerSubject)]
	if !ok {
		return identity.Identity{}, identity.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *Repository) Touch(ctx context.Context, provider, providerSubject string) (identity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	k := key(provider, providerSubject)
	if id, ok := r.byLinkage[k]; ok {
		rec := r.byID[id]
		rec.LastLoginAt = &now
		r.byID[id] = rec
		return rec, nil
	}

	rec := identity.Identity{
		ID: uuid.NewString(), Provider: provider, ProviderSubject: providerSubject,
		Status: identity.StatusActive, CreatedAt: now, LastLoginAt: &now,
	}
	r.byID[rec.ID] = rec
	r.byLinkage[k] = rec.ID
	return rec, nil
}

func (r *Repository) Disable(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok {
		return identity.ErrNotFound
	}
	rec.Status = identity.StatusDisabled
	r.byID[id] = rec
	return nil
}

func (r *Repository) LinkUser(ctx context.Context, identityID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[identityID]
	if !ok {
		return identity.ErrNotFound
	}
	rec.UserID = &userID
	r.byID[identityID] = rec
	return nil
}
