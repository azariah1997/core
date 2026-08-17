// Package postgres is the PostgreSQL-backed tenants.Repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/tenants"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const tenantColumns = "id, app_id, slug, name, status, created_at, updated_at"
const membershipColumns = "id, tenant_id, user_id, role, created_at"

// Create writes the tenant and its owner membership in one transaction -
// a tenant can never transiently exist with no owner.
func (r *Repository) Create(ctx context.Context, ownerUserID string, in tenants.CreateInput) (tenants.Tenant, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenants.Tenant{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var t tenants.Tenant
	err = tx.QueryRow(ctx,
		`INSERT INTO tenants (app_id, slug, name) VALUES ($1, $2, $3) RETURNING `+tenantColumns,
		in.AppID, in.Slug, in.Name,
	).Scan(&t.ID, &t.AppID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return tenants.Tenant{}, tenants.ErrSlugTaken
		}
		return tenants.Tenant{}, fmt.Errorf("insert tenant: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, 'owner')`,
		t.ID, ownerUserID,
	); err != nil {
		return tenants.Tenant{}, fmt.Errorf("insert owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return tenants.Tenant{}, fmt.Errorf("commit tx: %w", err)
	}
	return t, nil
}

func (r *Repository) Get(ctx context.Context, id string) (tenants.Tenant, error) {
	var t tenants.Tenant
	err := r.pool.QueryRow(ctx, `SELECT `+tenantColumns+` FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.AppID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenants.Tenant{}, tenants.ErrNotFound
		}
		return tenants.Tenant{}, fmt.Errorf("get tenant: %w", err)
	}
	return t, nil
}

func (r *Repository) ListForUser(ctx context.Context, userID string) ([]tenants.Tenant, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.id, t.app_id, t.slug, t.name, t.status, t.created_at, t.updated_at
		 FROM tenants t JOIN tenant_memberships m ON m.tenant_id = t.id
		 WHERE m.user_id = $1 ORDER BY t.created_at`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list tenants for user: %w", err)
	}
	defer rows.Close()

	var list []tenants.Tenant
	for rows.Next() {
		var t tenants.Tenant
		if err := rows.Scan(&t.ID, &t.AppID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}
	return list, nil
}

func (r *Repository) Update(ctx context.Context, id string, in tenants.UpdateInput) (tenants.Tenant, error) {
	setClauses := "updated_at = now()"
	args := []any{}
	if in.Name != nil {
		args = append(args, *in.Name)
		setClauses += fmt.Sprintf(", name = $%d", len(args))
	}
	if in.Status != nil {
		args = append(args, string(*in.Status))
		setClauses += fmt.Sprintf(", status = $%d", len(args))
	}
	args = append(args, id)
	query := fmt.Sprintf(`UPDATE tenants SET %s WHERE id = $%d RETURNING `+tenantColumns, setClauses, len(args))

	var t tenants.Tenant
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&t.ID, &t.AppID, &t.Slug, &t.Name, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenants.Tenant{}, tenants.ErrNotFound
		}
		return tenants.Tenant{}, fmt.Errorf("update tenant: %w", err)
	}
	return t, nil
}

func (r *Repository) GetMembership(ctx context.Context, tenantID, userID string) (tenants.Membership, error) {
	var m tenants.Membership
	err := r.pool.QueryRow(ctx,
		`SELECT `+membershipColumns+` FROM tenant_memberships WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenants.Membership{}, tenants.ErrMembershipNotFound
		}
		return tenants.Membership{}, fmt.Errorf("get membership: %w", err)
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, tenantID string) ([]tenants.Membership, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+membershipColumns+` FROM tenant_memberships WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var list []tenants.Membership
	for rows.Next() {
		var m tenants.Membership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate members: %w", err)
	}
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, tenantID string, in tenants.AddMemberInput) (tenants.Membership, error) {
	var m tenants.Membership
	err := r.pool.QueryRow(ctx,
		`INSERT INTO tenant_memberships (tenant_id, user_id, role) VALUES ($1, $2, $3) RETURNING `+membershipColumns,
		tenantID, in.UserID, string(in.Role),
	).Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return tenants.Membership{}, tenants.ErrAlreadyMember
		}
		return tenants.Membership{}, fmt.Errorf("add member: %w", err)
	}
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, tenantID, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM tenant_memberships WHERE tenant_id = $1 AND user_id = $2`, tenantID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tenants.ErrMembershipNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
