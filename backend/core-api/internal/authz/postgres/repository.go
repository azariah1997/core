// Package postgres is the PostgreSQL-backed authz.RoleRepository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/authz"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) RolesFor(ctx context.Context, userID string) ([]authz.Role, error) {
	rows, err := r.pool.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("query roles: %w", err)
	}
	defer rows.Close()

	var roles []authz.Role
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, authz.Role(role))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}
	return roles, nil
}

func (r *Repository) AssignRole(ctx context.Context, userID string, role authz.Role) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role) VALUES ($1, $2) ON CONFLICT (user_id, role) DO NOTHING`,
		userID, string(role))
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRole(ctx context.Context, userID string, role authz.Role) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role = $2`, userID, string(role))
	if err != nil {
		return fmt.Errorf("revoke role: %w", err)
	}
	return nil
}
