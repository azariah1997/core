// Package identity resolves an authenticated token's provider subject to
// the platform User ID everything else in the system (Applications,
// Tenants, Relationships, Groups) already addresses users by. This is
// deliberately narrow - a single read-only query against the identities
// table core-api's own identity module owns - not a duplicate of that
// module's domain logic. realtime-gateway shares the platform's one
// Postgres database by design (see docs/architecture/overview.md); it
// just never writes to a table it doesn't own.
package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotLinked = errors.New("identity has no linked platform user")

type Resolver struct {
	pool *pgxpool.Pool
}

func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{pool: pool}
}

// UserIDForSubject returns the platform user ID linked to a (provider,
// providerSubject) identity. Returns ErrNotLinked if the identity exists
// but has no linked user yet (shouldn't happen once EnsureForIdentity has
// run at least once via a core-api request) or doesn't exist at all.
func (r *Resolver) UserIDForSubject(ctx context.Context, provider, providerSubject string) (string, error) {
	var userID *string
	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM identities WHERE provider = $1 AND provider_subject = $2`,
		provider, providerSubject,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotLinked
		}
		return "", fmt.Errorf("resolve identity: %w", err)
	}
	if userID == nil {
		return "", ErrNotLinked
	}
	return *userID, nil
}
