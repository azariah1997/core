// Package memory is an in-memory users.Repository for tests.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
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

func (r *Repository) List(ctx context.Context, params users.ListParams) (users.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []users.User
	for _, u := range r.byID {
		if u.Status != users.StatusDeleted {
			all = append(all, u)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})

	start := 0
	if params.Cursor != "" {
		afterID, err := decodeCursor(params.Cursor)
		if err != nil {
			return users.ListResult{}, &users.ValidationError{Message: "invalid cursor"}
		}
		for i, u := range all {
			if u.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + params.Limit
	if end > len(all) {
		end = len(all)
	}
	if start > len(all) {
		start = len(all)
	}
	page := append([]users.User{}, all[start:end]...)

	result := users.ListResult{Items: page}
	if end < len(all) {
		result.NextCursor = encodeCursor(page[len(page)-1].ID)
	}
	return result, nil
}

func encodeCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func decodeCursor(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	return string(raw), err
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
