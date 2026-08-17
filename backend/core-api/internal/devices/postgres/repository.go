// Package postgres is the PostgreSQL-backed devices.Repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/devices"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const selectColumns = `id, user_id, coalesce(client_device_id, ''), platform, coalesce(os_version, ''),
	coalesce(app_version, ''), coalesce(locale, ''), coalesce(timezone, ''),
	(push_token IS NOT NULL AND push_token <> ''), session_status, last_seen_at, created_at`

// Register upserts on (user_id, client_device_id): a first-sight
// registration creates the row, a repeat one refreshes profile fields and
// LastActiveAt, and re-registering a previously revoked device reactivates
// it - exactly the "log back in on this device" case.
func (r *Repository) Register(ctx context.Context, userID string, in devices.RegisterInput) (devices.Device, error) {
	var pushToken any
	if in.PushToken != "" {
		pushToken = in.PushToken
	}
	var d devices.Device
	err := r.pool.QueryRow(ctx,
		`INSERT INTO devices (user_id, client_device_id, platform, os_version, app_version, locale, timezone, push_token, session_status, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', now())
		 ON CONFLICT (user_id, client_device_id) DO UPDATE SET
		   platform = EXCLUDED.platform,
		   os_version = EXCLUDED.os_version,
		   app_version = EXCLUDED.app_version,
		   locale = EXCLUDED.locale,
		   timezone = EXCLUDED.timezone,
		   push_token = COALESCE(EXCLUDED.push_token, devices.push_token),
		   session_status = 'active',
		   last_seen_at = now()
		 RETURNING `+selectColumns,
		userID, in.ClientDeviceID, in.Platform,
		nullIfEmpty(in.OSVersion), nullIfEmpty(in.AppVersion), nullIfEmpty(in.Locale), nullIfEmpty(in.Timezone), pushToken,
	).Scan(&d.ID, &d.UserID, &d.ClientDeviceID, &d.Platform, &d.OSVersion, &d.AppVersion,
		&d.Locale, &d.Timezone, &d.HasPushToken, &d.SessionStatus, &d.LastActiveAt, &d.CreatedAt)
	if err != nil {
		return devices.Device{}, fmt.Errorf("register device: %w", err)
	}
	return d, nil
}

func (r *Repository) List(ctx context.Context, userID string) ([]devices.Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+selectColumns+` FROM devices WHERE user_id = $1 AND session_status = 'active' ORDER BY last_seen_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var list []devices.Device
	for rows.Next() {
		var d devices.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.ClientDeviceID, &d.Platform, &d.OSVersion, &d.AppVersion,
			&d.Locale, &d.Timezone, &d.HasPushToken, &d.SessionStatus, &d.LastActiveAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return list, nil
}

// Revoke is scoped to userID as well as deviceID, so a caller can never
// revoke a device it doesn't own even if it guessed a valid device ID.
func (r *Repository) Revoke(ctx context.Context, userID, deviceID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE devices SET session_status = 'revoked' WHERE id = $1 AND user_id = $2 AND session_status = 'active'`,
		deviceID, userID)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return devices.ErrNotFound
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
