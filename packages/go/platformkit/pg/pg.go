// Package pg provides the platform's single way to obtain a PostgreSQL
// connection pool, so services never construct one ad hoc.
package pg

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect builds a pool and verifies connectivity before returning, so
// callers fail fast at startup rather than on the first query.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
