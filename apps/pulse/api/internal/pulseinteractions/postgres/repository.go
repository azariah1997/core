// Package postgres is the production pulseinteractions.Repository,
// against Pulse's own database.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const interactionColumns = "id, type, sender_id, receiver_id, started_at, ended_at, duration_ms, delivery_mode, status, created_at, updated_at"

func scanInteraction(row pgx.Row) (pulseinteractions.Interaction, error) {
	var i pulseinteractions.Interaction
	var typ, deliveryMode, status string
	err := row.Scan(&i.ID, &typ, &i.SenderID, &i.ReceiverID, &i.StartedAt, &i.EndedAt, &i.DurationMs, &deliveryMode, &status, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return pulseinteractions.Interaction{}, err
	}
	i.Type = pulseinteractions.Type(typ)
	i.DeliveryMode = pulseinteractions.DeliveryMode(deliveryMode)
	i.Status = pulseinteractions.Status(status)
	return i, nil
}

func (r *Repository) Create(ctx context.Context, i pulseinteractions.Interaction, clientRequestID string) (pulseinteractions.Interaction, error) {
	if clientRequestID != "" {
		row := r.pool.QueryRow(ctx, `SELECT `+interactionColumns+` FROM interactions WHERE sender_id = $1 AND client_request_id = $2`, i.SenderID, clientRequestID)
		existing, err := scanInteraction(row)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return pulseinteractions.Interaction{}, err
		}
	}

	var crid *string
	if clientRequestID != "" {
		crid = &clientRequestID
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO interactions (id, type, sender_id, receiver_id, delivery_mode, status, client_request_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING `+interactionColumns,
		i.ID, string(i.Type), i.SenderID, i.ReceiverID, string(i.DeliveryMode), string(i.Status), crid, i.CreatedAt)
	created, err := scanInteraction(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Lost an idempotency race to a concurrent identical retry -
			// fetch and return the row that won, never erroring the
			// caller for their own retry (spec §76).
			row := r.pool.QueryRow(ctx, `SELECT `+interactionColumns+` FROM interactions WHERE sender_id = $1 AND client_request_id = $2`, i.SenderID, clientRequestID)
			return scanInteraction(row)
		}
		return pulseinteractions.Interaction{}, err
	}
	return created, nil
}

func (r *Repository) Get(ctx context.Context, id string) (pulseinteractions.Interaction, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+interactionColumns+` FROM interactions WHERE id = $1`, id)
	i, err := scanInteraction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	return i, err
}

func (r *Repository) Start(ctx context.Context, id string, startedAt time.Time, deliveryMode pulseinteractions.DeliveryMode) (pulseinteractions.Interaction, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE interactions SET status = 'started', started_at = $2, delivery_mode = $3, updated_at = $2
		WHERE id = $1
		RETURNING `+interactionColumns,
		id, startedAt, string(deliveryMode))
	i, err := scanInteraction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	return i, err
}

// Stop computes duration_ms server-side from the row's own started_at
// in the same SQL statement (never trusting a value passed from Go,
// which itself never trusts a client-submitted value - spec §78).
func (r *Repository) Stop(ctx context.Context, id string, endedAt time.Time) (pulseinteractions.Interaction, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE interactions
		SET status = 'completed', ended_at = $2, duration_ms = (EXTRACT(EPOCH FROM ($2 - started_at)) * 1000)::INTEGER, updated_at = $2
		WHERE id = $1
		RETURNING `+interactionColumns,
		id, endedAt)
	i, err := scanInteraction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return pulseinteractions.Interaction{}, pulseinteractions.ErrNotFound
	}
	return i, err
}

func (r *Repository) ListForUser(ctx context.Context, userID string, limit int) ([]pulseinteractions.Interaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+interactionColumns+` FROM interactions
		WHERE sender_id = $1 OR receiver_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pulseinteractions.Interaction
	for rows.Next() {
		i, err := scanInteraction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
