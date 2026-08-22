// Package postgres is the production livetouch.Repository, against
// Pulse's own database.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const sessionColumns = "id, initiator_id, receiver_id, status, end_reason, delivery_mode, invited_at, accepted_at, ended_at, duration_ms, updated_at"

func scanSession(row pgx.Row) (livetouch.Session, error) {
	var s livetouch.Session
	var status, deliveryMode string
	var endReason *string
	err := row.Scan(&s.ID, &s.InitiatorID, &s.ReceiverID, &status, &endReason, &deliveryMode,
		&s.InvitedAt, &s.AcceptedAt, &s.EndedAt, &s.DurationMs, &s.UpdatedAt)
	if err != nil {
		return livetouch.Session{}, err
	}
	s.Status = livetouch.Status(status)
	s.DeliveryMode = livetouch.DeliveryMode(deliveryMode)
	if endReason != nil {
		r := livetouch.EndReason(*endReason)
		s.EndReason = &r
	}
	return s, nil
}

func (r *Repository) Create(ctx context.Context, s livetouch.Session) (livetouch.Session, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO live_touch_sessions (id, initiator_id, receiver_id, status, delivery_mode, invited_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING `+sessionColumns,
		s.ID, s.InitiatorID, s.ReceiverID, string(s.Status), string(s.DeliveryMode), s.InvitedAt)
	return scanSession(row)
}

func (r *Repository) Get(ctx context.Context, id string) (livetouch.Session, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM live_touch_sessions WHERE id = $1`, id)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	return s, err
}

func (r *Repository) Accept(ctx context.Context, id string, acceptedAt time.Time) (livetouch.Session, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE live_touch_sessions SET status = 'active', accepted_at = $2, updated_at = $2
		WHERE id = $1
		RETURNING `+sessionColumns,
		id, acceptedAt)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	return s, err
}

// End computes duration_ms server-side from the row's own accepted_at,
// in the same SQL statement (never trusting a value passed from Go,
// which itself never trusts a client-submitted value - spec §78's own
// "server timestamps are truth", applied here too). NULL for a session
// that was never accepted (accepted_at IS NULL), matching Go's own
// s.AcceptedAt == nil check.
func (r *Repository) End(ctx context.Context, id string, endedAt time.Time, reason livetouch.EndReason) (livetouch.Session, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE live_touch_sessions
		SET status = 'ended', end_reason = $2, ended_at = $3, updated_at = $3,
			duration_ms = CASE WHEN accepted_at IS NULL THEN NULL ELSE (EXTRACT(EPOCH FROM ($3 - accepted_at)) * 1000)::INTEGER END
		WHERE id = $1
		RETURNING `+sessionColumns,
		id, string(reason), endedAt)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return livetouch.Session{}, livetouch.ErrNotFound
	}
	return s, err
}
