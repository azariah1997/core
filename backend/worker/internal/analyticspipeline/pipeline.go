// Package analyticspipeline is Phase 23's "pipeline foundation for
// future ClickHouse/warehouse/data lake": it claims unflushed rows from
// core-api's analytics_events landing table, batches them as
// newline-delimited JSON, and writes each batch as one object in
// object storage - a real, common "data lake" pattern (S3 as the
// landing zone a ClickHouse S3 table function, Redshift Spectrum, or a
// warehouse COPY/LOAD job would point at next), not a simulation of
// one.
//
// This package cannot import core-api/internal/analytics - worker is a
// separate Go module, the same constraint documented since Phase 14's
// search indexer - so it defines its own minimal event shape below,
// read directly off the same columns via a plain SQL query rather than
// through core-api's Repository interface.
package analyticspipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sink is the narrow surface this pipeline needs from object storage -
// satisfied directly by *platformkit/blobstore.Store.
type Sink interface {
	PutObject(ctx context.Context, key string, body []byte, contentType string) error
}

// batchSize caps how many rows one poll claims - large enough that a
// busy platform doesn't fall behind, small enough that one failed batch
// (a transient S3 outage) doesn't hold an enormous transaction open.
const batchSize = 500

type Pipeline struct {
	pool *pgxpool.Pool
	sink Sink
}

func New(pool *pgxpool.Pool, sink Sink) *Pipeline {
	return &Pipeline{pool: pool, sink: sink}
}

// ndjsonEvent's field names deliberately match the roadmap's own
// example envelope (event_name, user_id, anonymous_id, app_id,
// session_id, timestamp, properties, context) verbatim - this is
// exactly the shape a downstream loader would read.
type ndjsonEvent struct {
	EventName   string         `json:"event_name"`
	UserID      string         `json:"user_id,omitempty"`
	AnonymousID string         `json:"anonymous_id,omitempty"`
	AppID       string         `json:"app_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Properties  map[string]any `json:"properties,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	IngestedAt  time.Time      `json:"ingested_at"`
}

// PollOnce claims up to batchSize unflushed rows (SELECT ... FOR UPDATE
// SKIP LOCKED, the same claiming pattern Phase 15's job queue uses),
// writes them as one NDJSON object, and only marks them flushed once
// that write has actually succeeded - holding the transaction open
// through the object storage call is the same tradeoff Phase 14's
// search indexer makes for a single (fast, local) external call, and
// guarantees a failed write leaves the rows unflushed for the next poll
// to retry rather than losing them.
func (p *Pipeline) PollOnce(ctx context.Context) (int, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT id, event_name, coalesce(user_id::text, ''), coalesce(anonymous_id, ''), coalesce(app_id::text, ''),
		        coalesce(session_id, ''), occurred_at, properties, context, ingested_at
		 FROM analytics_events WHERE flushed_at IS NULL ORDER BY ingested_at LIMIT $1 FOR UPDATE SKIP LOCKED`,
		batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim events: %w", err)
	}

	var ids []string
	var events []ndjsonEvent
	for rows.Next() {
		var id string
		var e ndjsonEvent
		var properties, evtContext []byte
		if err := rows.Scan(&id, &e.EventName, &e.UserID, &e.AnonymousID, &e.AppID, &e.SessionID, &e.Timestamp, &properties, &evtContext, &e.IngestedAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan event: %w", err)
		}
		if len(properties) > 0 {
			_ = json.Unmarshal(properties, &e.Properties)
		}
		if len(evtContext) > 0 {
			_ = json.Unmarshal(evtContext, &e.Context)
		}
		ids = append(ids, id)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate events: %w", err)
	}
	rows.Close()

	if len(ids) == 0 {
		return 0, nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return 0, fmt.Errorf("encode ndjson: %w", err)
		}
	}

	key := fmt.Sprintf("analytics/%s/%s.ndjson", time.Now().UTC().Format("2006/01/02"), uuid.NewString())
	if err := p.sink.PutObject(ctx, key, buf.Bytes(), "application/x-ndjson"); err != nil {
		return 0, fmt.Errorf("write batch to object storage: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE analytics_events SET flushed_at = now(), batch_ref = $1 WHERE id = ANY($2)`, key, ids); err != nil {
		return 0, fmt.Errorf("mark events flushed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(ids), nil
}
