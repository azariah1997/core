// Package postgres is the production pulseprefs.Repository, against
// Pulse's own database.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const prefsColumns = "user_id, notification_detail, haptic_intensity, updated_at"

func scanPreferences(row pgx.Row) (pulseprefs.Preferences, error) {
	var p pulseprefs.Preferences
	var detail string
	err := row.Scan(&p.UserID, &detail, &p.HapticIntensity, &p.UpdatedAt)
	if err != nil {
		return pulseprefs.Preferences{}, err
	}
	p.NotificationDetail = pulseprefs.NotificationDetail(detail)
	return p, nil
}

// Get returns DefaultPreferences(userID) when no row exists yet - no
// row is the normal state for most users, not a not-found error,
// mirroring Core's own QuietHours repository convention.
func (r *Repository) Get(ctx context.Context, userID string) (pulseprefs.Preferences, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+prefsColumns+` FROM pulse_preferences WHERE user_id = $1`, userID)
	p, err := scanPreferences(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseprefs.DefaultPreferences(userID), nil
	}
	return p, err
}

// Set upserts the caller's singleton preferences row.
func (r *Repository) Set(ctx context.Context, p pulseprefs.Preferences) (pulseprefs.Preferences, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO pulse_preferences (user_id, notification_detail, haptic_intensity, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			notification_detail = EXCLUDED.notification_detail, haptic_intensity = EXCLUDED.haptic_intensity, updated_at = EXCLUDED.updated_at
		RETURNING `+prefsColumns,
		p.UserID, string(p.NotificationDetail), p.HapticIntensity, p.UpdatedAt)
	return scanPreferences(row)
}
