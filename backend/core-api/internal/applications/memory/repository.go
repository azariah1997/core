// Package memory is an in-memory applications.Repository used by tests and
// by any caller that wants the Application Registry without a database.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/applications"
)

type Repository struct {
	mu     sync.Mutex
	byID   map[string]applications.Application
	bySlug map[string]string
}

func New() *Repository {
	return &Repository{byID: map[string]applications.Application{}, bySlug: map[string]string{}}
}

func (r *Repository) Create(ctx context.Context, in applications.CreateInput) (applications.Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.bySlug[in.Slug]; taken {
		return applications.Application{}, applications.ErrSlugTaken
	}
	now := time.Now().UTC()
	app := applications.Application{
		ID: uuid.NewString(), Slug: in.Slug, Name: in.Name,
		Status: applications.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	r.byID[app.ID] = app
	r.bySlug[app.Slug] = app.ID
	return app, nil
}

func (r *Repository) Get(ctx context.Context, id string) (applications.Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	app, ok := r.byID[id]
	if !ok {
		return applications.Application{}, applications.ErrNotFound
	}
	return app, nil
}

func (r *Repository) List(ctx context.Context, params applications.ListParams) (applications.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]applications.Application, 0, len(r.byID))
	for _, app := range r.byID {
		items = append(items, app)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	start := 0
	if params.Cursor != "" {
		for i, app := range items {
			if app.ID == params.Cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + params.Limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]

	result := applications.ListResult{Items: page}
	if end < len(items) {
		result.NextCursor = page[len(page)-1].ID
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, id string, in applications.UpdateInput) (applications.Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	app, ok := r.byID[id]
	if !ok {
		return applications.Application{}, applications.ErrNotFound
	}
	if in.Name != nil {
		app.Name = *in.Name
	}
	if in.Status != nil {
		app.Status = *in.Status
	}
	app.UpdatedAt = time.Now().UTC()
	r.byID[id] = app
	return app, nil
}
