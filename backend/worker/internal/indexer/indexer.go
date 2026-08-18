package indexer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

const batchSize = 20

type Indexer struct {
	pool     *pgxpool.Pool
	provider searchidx.Provider
	logger   *slog.Logger
}

func New(pool *pgxpool.Pool, provider searchidx.Provider, logger *slog.Logger) *Indexer {
	return &Indexer{pool: pool, provider: provider, logger: logger}
}

// PollOnce claims up to batchSize unpublished, recognized outbox events
// via SELECT ... FOR UPDATE SKIP LOCKED (safe under concurrent worker
// replicas, though only one runs locally), applies each to the search
// index, and marks it published - all in one transaction. Holding a
// Postgres row lock for the duration of an OpenSearch call is an
// acceptable tradeoff at this batch size for a local/single-replica
// indexer; a higher-throughput deployment would want a claim-then-process
// split instead.
func (idx *Indexer) PollOnce(ctx context.Context) (int, error) {
	tx, err := idx.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT id, event_type, payload FROM outbox_events
		 WHERE published_at IS NULL AND event_type = ANY($1)
		 ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED`,
		recognizedEventTypes, batchSize)
	if err != nil {
		return 0, fmt.Errorf("select unpublished events: %w", err)
	}

	type row struct {
		id        string
		eventType string
		payload   []byte
	}
	var claimed []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.eventType, &r.payload); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox event: %w", err)
		}
		claimed = append(claimed, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate outbox events: %w", err)
	}

	processed := 0
	for _, r := range claimed {
		d, err := buildDispatch(r.eventType, r.payload)
		if err != nil {
			idx.logger.Error("indexer: failed to parse event payload, marking published to avoid a poison-pill loop",
				"error", err, "eventId", r.id, "eventType", r.eventType)
			d = dispatch{action: actionSkip}
		}

		switch d.action {
		case actionIndex:
			if err := idx.provider.Index(ctx, d.doc); err != nil {
				return processed, fmt.Errorf("index document for event %s: %w", r.id, err)
			}
		case actionDelete:
			if err := idx.provider.Delete(ctx, d.delType, "", d.delID); err != nil {
				return processed, fmt.Errorf("delete document for event %s: %w", r.id, err)
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, r.id); err != nil {
			return processed, fmt.Errorf("mark event %s published: %w", r.id, err)
		}
		processed++
	}

	if err := tx.Commit(ctx); err != nil {
		return processed, fmt.Errorf("commit tx: %w", err)
	}
	return processed, nil
}
