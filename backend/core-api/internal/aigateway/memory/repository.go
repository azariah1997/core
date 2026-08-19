// Package memory is an in-memory aigateway.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
)

type Repository struct {
	mu          sync.Mutex
	completions []aigateway.Completion
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) RecordCompletion(ctx context.Context, c aigateway.Completion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.Text = "" // never persisted - see aigateway.Completion's doc comment
	r.completions = append(r.completions, c)
	return nil
}

func (r *Repository) ListCompletions(ctx context.Context, userID string, limit int) ([]aigateway.Completion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []aigateway.Completion
	for _, c := range r.completions {
		if c.UserID == userID {
			list = append(list, c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}
