// Package postgres is the production mood.Repository, against Pulse's
// own database.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/mood"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const moodColumns = "user_id, emoji, audience, allowed_viewer_ids, circle_id, created_at, expires_at, updated_at"

func scanMood(row pgx.Row) (mood.Mood, error) {
	var m mood.Mood
	var emoji, audience string
	err := row.Scan(&m.UserID, &emoji, &audience, &m.AllowedViewerIDs, &m.CircleID, &m.CreatedAt, &m.ExpiresAt, &m.UpdatedAt)
	if err != nil {
		return mood.Mood{}, err
	}
	m.Emoji = mood.Emoji(emoji)
	m.Audience = mood.Audience(audience)
	return m, nil
}

// Set upserts the caller's singleton Mood row - Today's Mood is always
// exactly one row per user, replaced wholesale, never a history table
// (spec §26: "historical Mood storage is a separate product decision").
func (r *Repository) Set(ctx context.Context, m mood.Mood) (mood.Mood, error) {
	// A nil Go slice (e.g. AudiencePrivate, or partner_only with no
	// current bond) would otherwise bind as SQL NULL, not an empty
	// array, violating allowed_viewer_ids' NOT NULL constraint.
	allowedViewerIDs := m.AllowedViewerIDs
	if allowedViewerIDs == nil {
		allowedViewerIDs = []string{}
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO moods (user_id, emoji, audience, allowed_viewer_ids, circle_id, created_at, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE SET
			emoji = EXCLUDED.emoji, audience = EXCLUDED.audience, allowed_viewer_ids = EXCLUDED.allowed_viewer_ids,
			circle_id = EXCLUDED.circle_id, created_at = EXCLUDED.created_at, expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at
		RETURNING `+moodColumns,
		m.UserID, string(m.Emoji), string(m.Audience), allowedViewerIDs, m.CircleID, m.CreatedAt, m.ExpiresAt, m.UpdatedAt)
	return scanMood(row)
}

// Get returns ErrNotFound both when the user has no Mood row and when
// their Mood has already expired - an expired Mood is functionally gone
// (spec §26), filtered server-side rather than requiring a background
// sweep job this phase.
func (r *Repository) Get(ctx context.Context, userID string) (mood.Mood, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+moodColumns+` FROM moods WHERE user_id = $1 AND expires_at > now()`, userID)
	m, err := scanMood(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return mood.Mood{}, mood.ErrNotFound
	}
	return m, err
}

func (r *Repository) Clear(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM moods WHERE user_id = $1`, userID)
	return err
}
