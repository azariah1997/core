// Package postgres is the PostgreSQL-backed analytics.Repository - the
// landing buffer, never the query surface. See package analytics' own
// doc comment.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/analytics"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) RecordBatch(ctx context.Context, events []analytics.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, e := range events {
		properties, err := json.Marshal(e.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties: %w", err)
		}
		context, err := json.Marshal(e.Context)
		if err != nil {
			return fmt.Errorf("marshal context: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO analytics_events (id, event_name, user_id, anonymous_id, app_id, session_id, occurred_at, properties, context, ingested_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			e.ID, e.EventName, nullIfEmpty(e.UserID), nullIfEmpty(e.AnonymousID), nullIfEmpty(e.AppID), nullIfEmpty(e.SessionID),
			e.OccurredAt, properties, context, e.IngestedAt); err != nil {
			return fmt.Errorf("record event: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListRecent(ctx context.Context, limit int) ([]analytics.Event, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, event_name, coalesce(user_id::text, ''), coalesce(anonymous_id, ''), coalesce(app_id::text, ''), coalesce(session_id, ''),
		        occurred_at, properties, context, ingested_at, flushed_at, coalesce(batch_ref, '')
		 FROM analytics_events ORDER BY ingested_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent events: %w", err)
	}
	defer rows.Close()

	var list []analytics.Event
	for rows.Next() {
		var e analytics.Event
		var properties, ctxBytes []byte
		if err := rows.Scan(&e.ID, &e.EventName, &e.UserID, &e.AnonymousID, &e.AppID, &e.SessionID,
			&e.OccurredAt, &properties, &ctxBytes, &e.IngestedAt, &e.FlushedAt, &e.BatchRef); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(properties) > 0 {
			if err := json.Unmarshal(properties, &e.Properties); err != nil {
				return nil, fmt.Errorf("unmarshal properties: %w", err)
			}
		}
		if len(ctxBytes) > 0 {
			if err := json.Unmarshal(ctxBytes, &e.Context); err != nil {
				return nil, fmt.Errorf("unmarshal context: %w", err)
			}
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
