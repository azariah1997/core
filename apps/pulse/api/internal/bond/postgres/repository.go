// Package postgres is the production bond.Repository - the one place
// the one-active-bond-per-user invariant is actually enforced (see
// Accept's doc comment), against Pulse's own database.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const bondColumns = "id, relationship_id, user_a, user_b, status, requested_at, accepted_at, ended_at, updated_at"

func scanBond(row pgx.Row) (bond.Bond, error) {
	var b bond.Bond
	var status string
	err := row.Scan(&b.ID, &b.RelationshipID, &b.UserAID, &b.UserBID, &status, &b.RequestedAt, &b.AcceptedAt, &b.EndedAt, &b.UpdatedAt)
	if err != nil {
		return bond.Bond{}, err
	}
	b.Status = bond.Status(status)
	return b, nil
}

func (r *Repository) Create(ctx context.Context, b bond.Bond) (bond.Bond, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO bonds (id, relationship_id, user_a, user_b, status, requested_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING `+bondColumns,
		b.ID, b.RelationshipID, b.UserAID, b.UserBID, string(b.Status), b.RequestedAt)
	return scanBond(row)
}

func (r *Repository) Get(ctx context.Context, id string) (bond.Bond, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+bondColumns+` FROM bonds WHERE id = $1`, id)
	b, err := scanBond(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bond.Bond{}, bond.ErrNotFound
	}
	return b, err
}

// Accept is the real one-active-bond-per-user enforcement: within a
// single transaction, it flips the bond to active and inserts both
// participants into bond_active_holders, whose PRIMARY KEY is user_id
// alone - one row per user, ever, regardless of which bond or which
// side (user_a/user_b) they're on. If either participant already holds
// an active bond with anyone (including a second, different concurrent
// Accept racing this one), the INSERT hits a real unique-constraint
// violation and the whole transaction rolls back atomically - this is
// Postgres's own concurrency guarantee, not an application-level
// check-then-write that a race could slip through.
func (r *Repository) Accept(ctx context.Context, id string) (bond.Bond, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bond.Bond{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `SELECT `+bondColumns+` FROM bonds WHERE id = $1 FOR UPDATE`, id)
	b, err := scanBond(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bond.Bond{}, bond.ErrNotFound
	}
	if err != nil {
		return bond.Bond{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE bonds SET status = 'active', accepted_at = $2, updated_at = $2 WHERE id = $1`, id, now); err != nil {
		return bond.Bond{}, err
	}

	_, err = tx.Exec(ctx, `INSERT INTO bond_active_holders (user_id, bond_id) VALUES ($1, $3), ($2, $3)`, b.UserAID, b.UserBID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return bond.Bond{}, bond.ErrAlreadyBonded
		}
		return bond.Bond{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bond.Bond{}, err
	}
	b.Status = bond.StatusActive
	b.AcceptedAt = &now
	b.UpdatedAt = now
	return b, nil
}

func (r *Repository) End(ctx context.Context, id string) (bond.Bond, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bond.Bond{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `SELECT `+bondColumns+` FROM bonds WHERE id = $1 FOR UPDATE`, id)
	b, err := scanBond(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bond.Bond{}, bond.ErrNotFound
	}
	if err != nil {
		return bond.Bond{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE bonds SET status = 'ended', ended_at = $2, updated_at = $2 WHERE id = $1`, id, now); err != nil {
		return bond.Bond{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bond_active_holders WHERE bond_id = $1`, id); err != nil {
		return bond.Bond{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bond.Bond{}, err
	}
	b.Status = bond.StatusEnded
	b.EndedAt = &now
	b.UpdatedAt = now
	return b, nil
}

func (r *Repository) ActiveForUser(ctx context.Context, userID string) (bond.Bond, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT b.id, b.relationship_id, b.user_a, b.user_b, b.status, b.requested_at, b.accepted_at, b.ended_at, b.updated_at
		FROM bonds b
		JOIN bond_active_holders h ON h.bond_id = b.id
		WHERE h.user_id = $1`, userID)
	b, err := scanBond(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return bond.Bond{}, bond.ErrNotFound
	}
	return b, err
}
