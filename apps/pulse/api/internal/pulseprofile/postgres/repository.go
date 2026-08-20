// Package postgres is the PostgreSQL-backed pulseprofile.Repository -
// against Pulse's own database (see apps/pulse/api/migrations), never
// Core's. Pulse's Postgres instance is logically separate from Core's,
// consistent with ADR-002's "domain APIs hide storage vendors and
// physical schemas."
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const profileColumns = "user_id, handle, visual_prefs, pulse_prefs, created_at, updated_at"

func scanProfile(row pgx.Row) (pulseprofile.Profile, error) {
	var p pulseprofile.Profile
	var visualPrefs, pulsePrefs []byte
	err := row.Scan(&p.UserID, &p.Handle, &visualPrefs, &pulsePrefs, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return pulseprofile.Profile{}, err
	}
	if len(visualPrefs) > 0 {
		_ = json.Unmarshal(visualPrefs, &p.VisualPrefs)
	}
	if len(pulsePrefs) > 0 {
		_ = json.Unmarshal(pulsePrefs, &p.PulsePrefs)
	}
	return p, nil
}

func (r *Repository) Create(ctx context.Context, p pulseprofile.Profile) (pulseprofile.Profile, error) {
	visualPrefs, _ := json.Marshal(p.VisualPrefs)
	pulsePrefs, _ := json.Marshal(p.PulsePrefs)
	row := r.pool.QueryRow(ctx, `
		INSERT INTO pulse_profiles (user_id, handle, visual_prefs, pulse_prefs, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+profileColumns,
		p.UserID, p.Handle, visualPrefs, pulsePrefs, p.CreatedAt, p.UpdatedAt)
	created, err := scanProfile(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return pulseprofile.Profile{}, pulseprofile.ErrHandleTaken
		}
		return pulseprofile.Profile{}, err
	}
	return created, nil
}

func (r *Repository) Get(ctx context.Context, userID string) (pulseprofile.Profile, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+profileColumns+` FROM pulse_profiles WHERE user_id = $1`, userID)
	p, err := scanProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseprofile.Profile{}, pulseprofile.ErrNotFound
	}
	return p, err
}

func (r *Repository) GetByHandle(ctx context.Context, handle string) (pulseprofile.Profile, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+profileColumns+` FROM pulse_profiles WHERE handle = $1`, handle)
	p, err := scanProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseprofile.Profile{}, pulseprofile.ErrNotFound
	}
	return p, err
}

func (r *Repository) Update(ctx context.Context, userID string, in pulseprofile.UpdateInput) (pulseprofile.Profile, error) {
	existing, err := r.Get(ctx, userID)
	if err != nil {
		return pulseprofile.Profile{}, err
	}
	if in.VisualPrefs != nil {
		existing.VisualPrefs = in.VisualPrefs
	}
	if in.PulsePrefs != nil {
		existing.PulsePrefs = in.PulsePrefs
	}
	visualPrefs, _ := json.Marshal(existing.VisualPrefs)
	pulsePrefs, _ := json.Marshal(existing.PulsePrefs)
	row := r.pool.QueryRow(ctx, `
		UPDATE pulse_profiles SET visual_prefs = $2, pulse_prefs = $3, updated_at = now()
		WHERE user_id = $1
		RETURNING `+profileColumns,
		userID, visualPrefs, pulsePrefs)
	return scanProfile(row)
}
