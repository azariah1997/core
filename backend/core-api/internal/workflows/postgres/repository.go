// Package postgres is the PostgreSQL-backed workflows.Repository - pure
// ownership/audit bookkeeping (workflow_id, run_id, type, created_by),
// never Temporal's own execution state.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/workflows"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, run workflows.WorkflowRun) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, run_id, type, created_by, created_at) VALUES ($1, $2, $3, $4, $5)`,
		run.WorkflowID, run.RunID, run.Type, nullIfEmpty(run.CreatedBy), run.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert workflow run: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, workflowID string) (workflows.WorkflowRun, error) {
	var run workflows.WorkflowRun
	var createdBy *string
	err := r.pool.QueryRow(ctx,
		`SELECT workflow_id, run_id, type, created_by, created_at FROM workflow_runs WHERE workflow_id = $1`, workflowID,
	).Scan(&run.WorkflowID, &run.RunID, &run.Type, &createdBy, &run.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workflows.WorkflowRun{}, workflows.ErrNotFound
		}
		return workflows.WorkflowRun{}, fmt.Errorf("get workflow run: %w", err)
	}
	if createdBy != nil {
		run.CreatedBy = *createdBy
	}
	return run, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
