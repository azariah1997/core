// Package postgres is the PostgreSQL-backed jobs.Repository. It shares
// the "jobs"/"job_attempts" tables with worker's internal/jobrunner,
// which claims and executes rows this package only ever inserts and
// reads - the same split as search's outbox-polling indexer, and for the
// same reason: worker and core-api are separate Go modules that can't
// import each other's internal packages, so the database table is the
// only contract between "enqueue" and "execute."
package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/jobs"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = "id, app_id, type, payload, status, run_at, recurrence_interval_seconds, max_attempts, attempts, created_by, created_at, updated_at"

// scanJob unpacks one `columns`-shaped row. appId/createdBy/
// recurrence_interval_seconds are nullable columns scanned through
// intermediate pointers, the same pattern every other module's
// Repository already uses for nullable fields.
func scanJob(row pgx.Row) (jobs.Job, error) {
	var j jobs.Job
	var appID, createdBy *string
	var recurrenceSeconds *int
	// jsonb scans directly into map[string]any via pgx's built-in codec,
	// the same pattern every other module's jsonb columns already use.
	err := row.Scan(&j.ID, &appID, &j.Type, &j.Payload, &j.Status, &j.RunAt,
		&recurrenceSeconds, &j.MaxAttempts, &j.Attempts, &createdBy, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return jobs.Job{}, err
	}
	if appID != nil {
		j.AppID = *appID
	}
	if createdBy != nil {
		j.CreatedBy = *createdBy
	}
	if recurrenceSeconds != nil {
		d := time.Duration(*recurrenceSeconds) * time.Second
		j.RecurrenceInterval = &d
	}
	return j, nil
}

func (r *Repository) Create(ctx context.Context, createdBy string, in jobs.EnqueueInput, runAt time.Time, maxAttempts int, recurrence *time.Duration) (jobs.Job, error) {
	payload, err := marshalJSON(in.Payload)
	if err != nil {
		return jobs.Job{}, err
	}
	var recurrenceSeconds *int
	if recurrence != nil {
		s := int(recurrence.Seconds())
		recurrenceSeconds = &s
	}

	j, err := scanJob(r.pool.QueryRow(ctx,
		`INSERT INTO jobs (app_id, type, payload, run_at, recurrence_interval_seconds, max_attempts, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+columns,
		nullIfEmpty(in.AppID), in.Type, payload, runAt, recurrenceSeconds, maxAttempts, nullIfEmpty(createdBy),
	))
	if err != nil {
		return jobs.Job{}, fmt.Errorf("insert job: %w", err)
	}
	return j, nil
}

func (r *Repository) Get(ctx context.Context, id string) (jobs.Job, error) {
	j, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM jobs WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.Job{}, jobs.ErrNotFound
		}
		return jobs.Job{}, fmt.Errorf("get job: %w", err)
	}
	return j, nil
}

func (r *Repository) ListForCaller(ctx context.Context, callerID string, params jobs.ListParams) (jobs.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+columns+` FROM jobs WHERE created_by = $1 ORDER BY created_at DESC, id DESC LIMIT $2`,
			callerID, params.Limit+1)
	} else {
		beforeCreated, beforeID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return jobs.ListResult{}, &jobs.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+columns+` FROM jobs
			 WHERE created_by = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			callerID, beforeCreated, beforeID, params.Limit+1)
	}
	if err != nil {
		return jobs.ListResult{}, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var items []jobs.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return jobs.ListResult{}, fmt.Errorf("scan job: %w", err)
		}
		items = append(items, j)
	}
	if err := rows.Err(); err != nil {
		return jobs.ListResult{}, fmt.Errorf("iterate jobs: %w", err)
	}

	result := jobs.ListResult{Items: items}
	if len(items) > params.Limit {
		last := items[params.Limit-1]
		result.Items = items[:params.Limit]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return result, nil
}

func (r *Repository) ListAttempts(ctx context.Context, jobID string) ([]jobs.JobAttempt, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, job_id, attempt_number, status, COALESCE(error,''), started_at, finished_at
		 FROM job_attempts WHERE job_id = $1 ORDER BY attempt_number`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list job attempts: %w", err)
	}
	defer rows.Close()

	var list []jobs.JobAttempt
	for rows.Next() {
		var a jobs.JobAttempt
		if err := rows.Scan(&a.ID, &a.JobID, &a.AttemptNumber, &a.Status, &a.Error, &a.StartedAt, &a.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan job attempt: %w", err)
		}
		list = append(list, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job attempts: %w", err)
	}
	return list, nil
}

// nullIfEmpty lets pgx bind a nil parameter (properly typed against the
// target uuid column via normal parameter binding) instead of embedding
// NULLIF(...) in the SQL text, which left Postgres unable to infer the
// right type for an empty string literal against a uuid column.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func marshalJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return b, nil
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}
