// Package postgres is the PostgreSQL-backed identity.Repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/identity"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const selectColumns = "id, user_id, provider, provider_subject, status, created_at, last_login_at"

func (r *Repository) GetByProviderSubject(ctx context.Context, provider, providerSubject string) (identity.Identity, error) {
	var id identity.Identity
	err := r.pool.QueryRow(ctx,
		`SELECT `+selectColumns+` FROM identities WHERE provider = $1 AND provider_subject = $2`,
		provider, providerSubject,
	).Scan(&id.ID, &id.UserID, &id.Provider, &id.ProviderSubject, &id.Status, &id.CreatedAt, &id.LastLoginAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Identity{}, identity.ErrNotFound
		}
		return identity.Identity{}, fmt.Errorf("get identity: %w", err)
	}
	return id, nil
}

// Touch upserts atomically: a first-sight login creates the linkage row, a
// repeat login just advances last_login_at. One statement, no race between
// "does it exist" and "create it".
func (r *Repository) Touch(ctx context.Context, provider, providerSubject string) (identity.Identity, error) {
	var id identity.Identity
	err := r.pool.QueryRow(ctx,
		`INSERT INTO identities (provider, provider_subject, last_login_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (provider, provider_subject)
		 DO UPDATE SET last_login_at = now()
		 RETURNING `+selectColumns,
		provider, providerSubject,
	).Scan(&id.ID, &id.UserID, &id.Provider, &id.ProviderSubject, &id.Status, &id.CreatedAt, &id.LastLoginAt)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("touch identity: %w", err)
	}
	return id, nil
}

func (r *Repository) Disable(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE identities SET status = 'disabled' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("disable identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}

func (r *Repository) LinkUser(ctx context.Context, identityID, userID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE identities SET user_id = $1 WHERE id = $2`, userID, identityID)
	if err != nil {
		return fmt.Errorf("link user to identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrNotFound
	}
	return nil
}
