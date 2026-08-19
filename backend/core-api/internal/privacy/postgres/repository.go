// Package postgres is the PostgreSQL-backed privacy.Repository.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/privacy"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) RecordConsent(ctx context.Context, c privacy.Consent) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO privacy_consents (id, user_id, purpose, granted, version, recorded_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.UserID, c.Purpose, c.Granted, c.Version, c.RecordedAt)
	if err != nil {
		return fmt.Errorf("record consent: %w", err)
	}
	return nil
}

func (r *Repository) ListConsent(ctx context.Context, userID string) ([]privacy.Consent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, purpose, granted, version, recorded_at FROM privacy_consents WHERE user_id = $1 ORDER BY recorded_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list consent: %w", err)
	}
	defer rows.Close()

	var list []privacy.Consent
	for rows.Next() {
		var c privacy.Consent
		if err := rows.Scan(&c.ID, &c.UserID, &c.Purpose, &c.Granted, &c.Version, &c.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan consent: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (r *Repository) GetPreferences(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM privacy_preferences WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("get preferences: %w", err)
	}
	defer rows.Close()

	prefs := map[string]bool{}
	for rows.Next() {
		var k string
		var v bool
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		prefs[k] = v
	}
	return prefs, rows.Err()
}

func (r *Repository) SetPreferences(ctx context.Context, userID string, prefs map[string]bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for key, value := range prefs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO privacy_preferences (user_id, key, value, updated_at) VALUES ($1, $2, $3, now())
			 ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
			userID, key, value); err != nil {
			return fmt.Errorf("set preference %q: %w", key, err)
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpsertRetentionPolicy(ctx context.Context, p privacy.RetentionPolicy) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO retention_policies (id, app_id, resource_type, retention_days, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (app_id, resource_type) DO UPDATE SET
		   retention_days = EXCLUDED.retention_days, updated_at = EXCLUDED.updated_at`,
		p.ID, nullIfEmpty(p.AppID), p.ResourceType, p.RetentionDays, nullIfEmpty(p.CreatedBy), p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert retention policy: %w", err)
	}
	return nil
}

func (r *Repository) ListRetentionPolicies(ctx context.Context) ([]privacy.RetentionPolicy, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, coalesce(app_id::text, ''), resource_type, retention_days, coalesce(created_by::text, ''), created_at, updated_at
		 FROM retention_policies ORDER BY resource_type`)
	if err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	defer rows.Close()

	var list []privacy.RetentionPolicy
	for rows.Next() {
		var p privacy.RetentionPolicy
		if err := rows.Scan(&p.ID, &p.AppID, &p.ResourceType, &p.RetentionDays, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan retention policy: %w", err)
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *Repository) CreateExportRequest(ctx context.Context, req privacy.ExportRequest) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO data_export_requests (id, user_id, status, requested_at) VALUES ($1, $2, $3, $4)`,
		req.ID, req.UserID, req.Status, req.RequestedAt)
	if err != nil {
		return fmt.Errorf("create export request: %w", err)
	}
	return nil
}

func (r *Repository) SetExportWorkflowRef(ctx context.Context, id, workflowID, runID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_requests SET workflow_id = $2, run_id = $3, status = 'running' WHERE id = $1`,
		id, workflowID, runID)
	if err != nil {
		return fmt.Errorf("set export workflow ref: %w", err)
	}
	return nil
}

func (r *Repository) CompleteExportRequest(ctx context.Context, id string, status privacy.RequestStatus, objectKey, errMsg string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_requests SET status = $2, object_key = $3, error = $4, completed_at = now() WHERE id = $1`,
		id, status, nullIfEmpty(objectKey), nullIfEmpty(errMsg))
	if err != nil {
		return fmt.Errorf("complete export request: %w", err)
	}
	return nil
}

func (r *Repository) GetExportRequest(ctx context.Context, id string) (privacy.ExportRequest, error) {
	var req privacy.ExportRequest
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, coalesce(workflow_id, ''), coalesce(run_id, ''), coalesce(object_key, ''), coalesce(error, ''), requested_at, completed_at
		 FROM data_export_requests WHERE id = $1`, id,
	).Scan(&req.ID, &req.UserID, &req.Status, &req.WorkflowID, &req.RunID, &req.ObjectKey, &req.Error, &req.RequestedAt, &req.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return privacy.ExportRequest{}, privacy.ErrNotFound
		}
		return privacy.ExportRequest{}, fmt.Errorf("get export request: %w", err)
	}
	return req, nil
}

func (r *Repository) CreateDeletionRequest(ctx context.Context, req privacy.DeletionRequest) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO data_deletion_requests (id, user_id, status, requested_at) VALUES ($1, $2, $3, $4)`,
		req.ID, req.UserID, req.Status, req.RequestedAt)
	if err != nil {
		return fmt.Errorf("create deletion request: %w", err)
	}
	return nil
}

func (r *Repository) SetDeletionWorkflowRef(ctx context.Context, id, workflowID, runID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_deletion_requests SET workflow_id = $2, run_id = $3, status = 'running' WHERE id = $1`,
		id, workflowID, runID)
	if err != nil {
		return fmt.Errorf("set deletion workflow ref: %w", err)
	}
	return nil
}

func (r *Repository) CompleteDeletionRequest(ctx context.Context, id string, status privacy.RequestStatus, results map[string]any) error {
	body, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal deletion results: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE data_deletion_requests SET status = $2, results = $3, completed_at = now() WHERE id = $1`,
		id, status, body)
	if err != nil {
		return fmt.Errorf("complete deletion request: %w", err)
	}
	return nil
}

func (r *Repository) GetDeletionRequest(ctx context.Context, id string) (privacy.DeletionRequest, error) {
	var req privacy.DeletionRequest
	var results []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, coalesce(workflow_id, ''), coalesce(run_id, ''), results, coalesce(error, ''), requested_at, completed_at
		 FROM data_deletion_requests WHERE id = $1`, id,
	).Scan(&req.ID, &req.UserID, &req.Status, &req.WorkflowID, &req.RunID, &results, &req.Error, &req.RequestedAt, &req.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return privacy.DeletionRequest{}, privacy.ErrNotFound
		}
		return privacy.DeletionRequest{}, fmt.Errorf("get deletion request: %w", err)
	}
	if len(results) > 0 {
		if err := json.Unmarshal(results, &req.Results); err != nil {
			return privacy.DeletionRequest{}, fmt.Errorf("unmarshal deletion results: %w", err)
		}
	}
	return req, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
