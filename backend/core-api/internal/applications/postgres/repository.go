// Package postgres is the PostgreSQL-backed applications.Repository. Writes
// use the transactional outbox pattern: the domain row and its outbox event
// are written in the same transaction, so a Kafka publish failure can never
// leave an application without a corresponding event (or vice versa) - a
// worker polling outbox_events owns actually publishing to Kafka.
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const selectColumns = "id, slug, name, status, created_at, updated_at"

func (r *Repository) Create(ctx context.Context, in applications.CreateInput) (applications.Application, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return applications.Application{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var app applications.Application
	err = tx.QueryRow(ctx,
		`INSERT INTO applications (slug, name) VALUES ($1, $2) RETURNING `+selectColumns,
		in.Slug, in.Name,
	).Scan(&app.ID, &app.Slug, &app.Name, &app.Status, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return applications.Application{}, applications.ErrSlugTaken
		}
		return applications.Application{}, fmt.Errorf("insert application: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, "application.created", app); err != nil {
		return applications.Application{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return applications.Application{}, fmt.Errorf("commit tx: %w", err)
	}
	return app, nil
}

func (r *Repository) Get(ctx context.Context, id string) (applications.Application, error) {
	var app applications.Application
	err := r.pool.QueryRow(ctx,
		`SELECT `+selectColumns+` FROM applications WHERE id = $1`, id,
	).Scan(&app.ID, &app.Slug, &app.Name, &app.Status, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return applications.Application{}, applications.ErrNotFound
		}
		return applications.Application{}, fmt.Errorf("get application: %w", err)
	}
	return app, nil
}

func (r *Repository) List(ctx context.Context, params applications.ListParams) (applications.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+selectColumns+` FROM applications ORDER BY created_at, id LIMIT $1`,
			params.Limit+1)
	} else {
		afterCreated, afterID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return applications.ListResult{}, &applications.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+selectColumns+` FROM applications
			 WHERE (created_at, id) > ($1, $2)
			 ORDER BY created_at, id LIMIT $3`,
			afterCreated, afterID, params.Limit+1)
	}
	if err != nil {
		return applications.ListResult{}, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	var items []applications.Application
	for rows.Next() {
		var app applications.Application
		if err := rows.Scan(&app.ID, &app.Slug, &app.Name, &app.Status, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return applications.ListResult{}, fmt.Errorf("scan application: %w", err)
		}
		items = append(items, app)
	}
	if err := rows.Err(); err != nil {
		return applications.ListResult{}, fmt.Errorf("iterate applications: %w", err)
	}

	result := applications.ListResult{Items: items}
	if len(items) > params.Limit {
		last := items[params.Limit-1]
		result.Items = items[:params.Limit]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, id string, in applications.UpdateInput) (applications.Application, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return applications.Application{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses := []string{"updated_at = now()"}
	args := []any{}
	if in.Name != nil {
		args = append(args, *in.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if in.Status != nil {
		args = append(args, string(*in.Status))
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, id)
	query := fmt.Sprintf(`UPDATE applications SET %s WHERE id = $%d RETURNING `+selectColumns,
		strings.Join(setClauses, ", "), len(args))

	var app applications.Application
	err = tx.QueryRow(ctx, query, args...).
		Scan(&app.ID, &app.Slug, &app.Name, &app.Status, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return applications.Application{}, applications.ErrNotFound
		}
		return applications.Application{}, fmt.Errorf("update application: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, "application.updated", app); err != nil {
		return applications.Application{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return applications.Application{}, fmt.Errorf("commit tx: %w", err)
	}
	return app, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, eventType string, app applications.Application) error {
	payload, err := json.Marshal(map[string]any{
		"id": app.ID, "slug": app.Slug, "name": app.Name, "status": app.Status,
		"createdAt": app.CreatedAt, "updatedAt": app.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('application', $1, $2, 1, $3, $4)`,
		app.ID, eventType, payload, nullIfEmpty(correlation.FromContext(ctx)),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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
