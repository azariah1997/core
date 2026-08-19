package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/packages/go/platformkit/metrics"
)

// ReportStats polls pool.Stat() on a ticker and records a real
// db_pool_connections snapshot (acquired/idle/total/max) for the
// "DB connections" dashboard - pgx is deliberately kept out of
// platformkit/metrics itself (see PoolStats' own doc comment there),
// so the actual driver-specific read lives here, next to Connect. The
// returned func stops the ticker; callers should defer it.
func ReportStats(ctx context.Context, service string, pool *pgxpool.Pool, interval time.Duration) func() {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	report := func() {
		s := pool.Stat()
		metrics.SetDBPoolStats(service, metrics.PoolStats{
			AcquiredConns: s.AcquiredConns(),
			IdleConns:     s.IdleConns(),
			TotalConns:    s.TotalConns(),
			MaxConns:      s.MaxConns(),
		})
	}

	go func() {
		report() // an immediate first reading, not just after the first tick
		for {
			select {
			case <-ticker.C:
				report()
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(done)
	}
}
