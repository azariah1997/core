// Package postgres is the PostgreSQL-backed users.Repository, using the
// same transactional outbox pattern as applications and identity.
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

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const selectColumns = "id, display_name, coalesce(avatar_ref, ''), locale, timezone, status, created_at, updated_at"

func (r *Repository) Create(ctx context.Context, in users.CreateInput) (users.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return users.User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u users.User
	err = tx.QueryRow(ctx,
		`INSERT INTO users (display_name, locale, timezone) VALUES ($1, $2, $3) RETURNING `+selectColumns,
		in.DisplayName, in.Locale, in.Timezone,
	).Scan(&u.ID, &u.DisplayName, &u.AvatarRef, &u.Locale, &u.Timezone, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return users.User{}, fmt.Errorf("insert user: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, "user.created", u); err != nil {
		return users.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return users.User{}, fmt.Errorf("commit tx: %w", err)
	}
	return u, nil
}

func (r *Repository) Get(ctx context.Context, id string) (users.User, error) {
	var u users.User
	err := r.pool.QueryRow(ctx,
		`SELECT `+selectColumns+` FROM users WHERE id = $1 AND status <> 'deleted'`, id,
	).Scan(&u.ID, &u.DisplayName, &u.AvatarRef, &u.Locale, &u.Timezone, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return users.User{}, users.ErrNotFound
		}
		return users.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (r *Repository) Update(ctx context.Context, id string, in users.UpdateInput) (users.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return users.User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses := []string{"updated_at = now()"}
	args := []any{}
	if in.DisplayName != nil {
		args = append(args, *in.DisplayName)
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", len(args)))
	}
	if in.AvatarRef != nil {
		args = append(args, *in.AvatarRef)
		setClauses = append(setClauses, fmt.Sprintf("avatar_ref = $%d", len(args)))
	}
	if in.Locale != nil {
		args = append(args, *in.Locale)
		setClauses = append(setClauses, fmt.Sprintf("locale = $%d", len(args)))
	}
	if in.Timezone != nil {
		args = append(args, *in.Timezone)
		setClauses = append(setClauses, fmt.Sprintf("timezone = $%d", len(args)))
	}
	if in.Status != nil {
		args = append(args, string(*in.Status))
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, id)
	query := fmt.Sprintf(`UPDATE users SET %s WHERE id = $%d AND status <> 'deleted' RETURNING `+selectColumns,
		strings.Join(setClauses, ", "), len(args))

	var u users.User
	err = tx.QueryRow(ctx, query, args...).
		Scan(&u.ID, &u.DisplayName, &u.AvatarRef, &u.Locale, &u.Timezone, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return users.User{}, users.ErrNotFound
		}
		return users.User{}, fmt.Errorf("update user: %w", err)
	}

	eventType := "user.updated"
	if in.Status != nil && *in.Status == users.StatusDeactivated {
		eventType = "user.deactivated"
	}
	if err := insertOutboxEvent(ctx, tx, eventType, u); err != nil {
		return users.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return users.User{}, fmt.Errorf("commit tx: %w", err)
	}
	return u, nil
}

func (r *Repository) Delete(ctx context.Context, id string) (users.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return users.User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u users.User
	err = tx.QueryRow(ctx,
		`UPDATE users SET status = 'deleted', updated_at = now() WHERE id = $1 AND status <> 'deleted'
		 RETURNING `+selectColumns,
		id,
	).Scan(&u.ID, &u.DisplayName, &u.AvatarRef, &u.Locale, &u.Timezone, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return users.User{}, users.ErrNotFound
		}
		return users.User{}, fmt.Errorf("delete user: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, "user.deleted", u); err != nil {
		return users.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return users.User{}, fmt.Errorf("commit tx: %w", err)
	}
	return u, nil
}

// List excludes deleted users - a browsable admin listing showing rows
// that no longer exist from any normal API's perspective would be
// actively misleading, the same "deleted is gone" rule Get already
// enforces.
func (r *Repository) List(ctx context.Context, params users.ListParams) (users.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+selectColumns+` FROM users WHERE status <> 'deleted' ORDER BY created_at, id LIMIT $1`,
			params.Limit+1)
	} else {
		afterCreated, afterID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return users.ListResult{}, &users.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+selectColumns+` FROM users
			 WHERE status <> 'deleted' AND (created_at, id) > ($1, $2)
			 ORDER BY created_at, id LIMIT $3`,
			afterCreated, afterID, params.Limit+1)
	}
	if err != nil {
		return users.ListResult{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var items []users.User
	for rows.Next() {
		var u users.User
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.AvatarRef, &u.Locale, &u.Timezone, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return users.ListResult{}, fmt.Errorf("scan user: %w", err)
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return users.ListResult{}, fmt.Errorf("iterate users: %w", err)
	}

	result := users.ListResult{Items: items}
	if len(items) > params.Limit {
		result.Items = items[:params.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return result, nil
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

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, eventType string, u users.User) error {
	payload, err := json.Marshal(map[string]any{
		"id": u.ID, "displayName": u.DisplayName, "status": u.Status,
		"createdAt": u.CreatedAt, "updatedAt": u.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('user', $1, $2, 1, $3, $4)`,
		u.ID, eventType, payload, nullIfEmpty(correlation.FromContext(ctx)),
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
