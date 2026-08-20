// Package postgres is the PostgreSQL-backed
// pulseconnections.ClassificationRepository, against Pulse's own
// database. The connection lifecycle itself (request/accept/decline/
// remove) has no table here at all - it lives entirely in Core's
// relationships storage; this package only ever stores the Pulse-owned
// Friend/Close-Friend overlay.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Set(ctx context.Context, relationshipID, ownerUserID string, c pulseconnections.Classification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO pulse_connection_classifications (relationship_id, owner_user_id, classification, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (relationship_id, owner_user_id) DO UPDATE SET classification = $3, updated_at = now()`,
		relationshipID, ownerUserID, c)
	return err
}

func (r *Repository) Get(ctx context.Context, relationshipID, ownerUserID string) (pulseconnections.Classification, error) {
	var c pulseconnections.Classification
	err := r.pool.QueryRow(ctx,
		`SELECT classification FROM pulse_connection_classifications WHERE relationship_id = $1 AND owner_user_id = $2`,
		relationshipID, ownerUserID).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseconnections.ClassificationFriend, nil
	}
	if err != nil {
		return "", err
	}
	return c, nil
}

func (r *Repository) ListForOwner(ctx context.Context, ownerUserID string) (map[string]pulseconnections.Classification, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT relationship_id, classification FROM pulse_connection_classifications WHERE owner_user_id = $1`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]pulseconnections.Classification{}
	for rows.Next() {
		var relationshipID string
		var c pulseconnections.Classification
		if err := rows.Scan(&relationshipID, &c); err != nil {
			return nil, err
		}
		out[relationshipID] = c
	}
	return out, rows.Err()
}
