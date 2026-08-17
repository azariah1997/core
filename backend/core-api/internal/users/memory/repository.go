// Package memory is an in-memory users.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/users"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]users.User
}

func New() *Repository {
	return &Repository{byID: map[string]users.User{}}
}

func (r *Repository) Create(ctx context.Context, in users.CreateInput) (users.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	u := users.User{
		ID: uuid.NewString(), DisplayName: in.DisplayName, Locale: in.Locale, Timezone: in.Timezone,
		Status: users.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	r.byID[u.ID] = u
	return u, nil
}

func (r *Repository) Get(ctx context.Context, id string) (users.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok || u.Status == users.StatusDeleted {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}

func (r *Repository) Update(ctx context.Context, id string, in users.UpdateInput) (users.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok || u.Status == users.StatusDeleted {
		return users.User{}, users.ErrNotFound
	}
	if in.DisplayName != nil {
		u.DisplayName = *in.DisplayName
	}
	if in.AvatarRef != nil {
		u.AvatarRef = *in.AvatarRef
	}
	if in.Locale != nil {
		u.Locale = *in.Locale
	}
	if in.Timezone != nil {
		u.Timezone = *in.Timezone
	}
	if in.Status != nil {
		u.Status = *in.Status
	}
	u.UpdatedAt = time.Now().UTC()
	r.byID[id] = u
	return u, nil
}

func (r *Repository) Delete(ctx context.Context, id string) (users.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok || u.Status == users.StatusDeleted {
		return users.User{}, users.ErrNotFound
	}
	u.Status = users.StatusDeleted
	u.UpdatedAt = time.Now().UTC()
	r.byID[id] = u
	return u, nil
}
