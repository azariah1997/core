// Package memory is an in-memory workflows.Repository for tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/core-platform/backend/core-api/internal/workflows"
)

type Repository struct {
	mu   sync.Mutex
	runs map[string]workflows.WorkflowRun
}

func New() *Repository {
	return &Repository{runs: map[string]workflows.WorkflowRun{}}
}

func (r *Repository) Create(ctx context.Context, run workflows.WorkflowRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.WorkflowID] = run
	return nil
}

func (r *Repository) Get(ctx context.Context, workflowID string) (workflows.WorkflowRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[workflowID]
	if !ok {
		return workflows.WorkflowRun{}, workflows.ErrNotFound
	}
	return run, nil
}
