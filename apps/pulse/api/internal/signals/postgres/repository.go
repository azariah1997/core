// Package postgres is the production signals.Repository, against
// Pulse's own database.
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/apps/pulse/api/internal/signals"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const signalColumns = "id, owner_user_id, target_user_id, label, segments, created_at"

// segmentRow mirrors signals.Segment for JSON (de)serialization into the
// segments JSONB column - Postgres has no notion of this Go type.
type segmentRow struct {
	Type       string `json:"type"`
	DurationMs int    `json:"durationMs"`
}

func scanSignal(row pgx.Row) (signals.Signal, error) {
	var s signals.Signal
	var segmentsJSON []byte
	err := row.Scan(&s.ID, &s.OwnerUserID, &s.TargetUserID, &s.Label, &segmentsJSON, &s.CreatedAt)
	if err != nil {
		return signals.Signal{}, err
	}
	var rows []segmentRow
	if err := json.Unmarshal(segmentsJSON, &rows); err != nil {
		return signals.Signal{}, err
	}
	s.Segments = make([]signals.Segment, 0, len(rows))
	for _, r := range rows {
		s.Segments = append(s.Segments, signals.Segment{Type: signals.SegmentType(r.Type), DurationMs: r.DurationMs})
	}
	return s, nil
}

func (r *Repository) Create(ctx context.Context, s signals.Signal) (signals.Signal, error) {
	rows := make([]segmentRow, 0, len(s.Segments))
	for _, seg := range s.Segments {
		rows = append(rows, segmentRow{Type: string(seg.Type), DurationMs: seg.DurationMs})
	}
	segmentsJSON, err := json.Marshal(rows)
	if err != nil {
		return signals.Signal{}, err
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO custom_signals (id, owner_user_id, target_user_id, label, segments, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+signalColumns,
		s.ID, s.OwnerUserID, s.TargetUserID, s.Label, segmentsJSON, s.CreatedAt)
	return scanSignal(row)
}

func (r *Repository) Get(ctx context.Context, id string) (signals.Signal, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+signalColumns+` FROM custom_signals WHERE id = $1`, id)
	s, err := scanSignal(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return signals.Signal{}, signals.ErrNotFound
	}
	return s, err
}

func (r *Repository) ListMine(ctx context.Context, ownerUserID string) ([]signals.Signal, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+signalColumns+` FROM custom_signals WHERE owner_user_id = $1 ORDER BY created_at DESC`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []signals.Signal
	for rows.Next() {
		s, err := scanSignal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM custom_signals WHERE id = $1`, id)
	return err
}
