// Package postgres is the production moments.Repository, against
// Pulse's own database.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/moments"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const momentColumns = "id, interaction_id, saved_by_user_id, other_user_id, interaction_type, duration_ms, pattern, occurred_at, saved_at"

func scanMoment(row pgx.Row) (moments.Moment, error) {
	var m moments.Moment
	err := row.Scan(&m.ID, &m.InteractionID, &m.SavedByUserID, &m.OtherUserID, &m.InteractionType,
		&m.DurationMs, &m.Pattern, &m.OccurredAt, &m.SavedAt)
	if err != nil {
		return moments.Moment{}, err
	}
	return m, nil
}

// Save is idempotent on (saved_by_user_id, interaction_id) via a real
// unique constraint - a retry (or the same participant saving the same
// interaction twice) returns the original row rather than a duplicate,
// the same real-constraint-backed idempotency pulse-interactions' own
// client_request_id uses.
func (r *Repository) Save(ctx context.Context, m moments.Moment) (moments.Moment, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO moments (id, interaction_id, saved_by_user_id, other_user_id, interaction_type, duration_ms, pattern, occurred_at, saved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (saved_by_user_id, interaction_id) DO UPDATE SET saved_at = moments.saved_at
		RETURNING `+momentColumns,
		m.ID, m.InteractionID, m.SavedByUserID, m.OtherUserID, m.InteractionType, m.DurationMs, m.Pattern, m.OccurredAt, m.SavedAt)
	return scanMoment(row)
}

func (r *Repository) Get(ctx context.Context, id string) (moments.Moment, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+momentColumns+` FROM moments WHERE id = $1`, id)
	m, err := scanMoment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return moments.Moment{}, moments.ErrNotFound
	}
	return m, err
}

func (r *Repository) ListMine(ctx context.Context, userID string, limit int) ([]moments.Moment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+momentColumns+` FROM moments
		WHERE saved_by_user_id = $1
		ORDER BY saved_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []moments.Moment
	for rows.Next() {
		m, err := scanMoment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM moments WHERE id = $1`, id)
	return err
}
