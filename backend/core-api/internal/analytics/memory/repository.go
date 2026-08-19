// Package memory is an in-memory analytics.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/example/core-platform/backend/core-api/internal/analytics"
)

type Repository struct {
	mu     sync.Mutex
	events []analytics.Event
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) RecordBatch(ctx context.Context, events []analytics.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return nil
}

func (r *Repository) ListRecent(ctx context.Context, limit int) ([]analytics.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := append([]analytics.Event{}, r.events...)
	sort.Slice(list, func(i, j int) bool { return list[i].IngestedAt.After(list[j].IngestedAt) })
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}
