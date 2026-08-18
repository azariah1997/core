// Package postgres is the PostgreSQL-backed groups.Repository.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/groups"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const groupColumns = "id, app_id, name, status, metadata, created_at, updated_at"
const memberColumns = "id, group_id, user_id, role, is_manager, created_at"

// Create writes the group and its creator's manager membership in one
// transaction - a group can never transiently exist with no manager.
func (r *Repository) Create(ctx context.Context, creatorUserID string, in groups.CreateInput) (groups.Group, error) {
	metadata, err := marshalMetadata(in.Metadata)
	if err != nil {
		return groups.Group{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return groups.Group{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var g groups.Group
	err = tx.QueryRow(ctx,
		`INSERT INTO groups (app_id, name, metadata) VALUES ($1, $2, $3) RETURNING `+groupColumns,
		in.AppID, in.Name, metadata,
	).Scan(&g.ID, &g.AppID, &g.Name, &g.Status, &g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return groups.Group{}, fmt.Errorf("insert group: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO group_members (group_id, user_id, role, is_manager) VALUES ($1, $2, 'owner', true)`,
		g.ID, creatorUserID,
	); err != nil {
		return groups.Group{}, fmt.Errorf("insert creator membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return groups.Group{}, fmt.Errorf("commit tx: %w", err)
	}
	return g, nil
}

func (r *Repository) Get(ctx context.Context, id string) (groups.Group, error) {
	var g groups.Group
	err := r.pool.QueryRow(ctx, `SELECT `+groupColumns+` FROM groups WHERE id = $1`, id).
		Scan(&g.ID, &g.AppID, &g.Name, &g.Status, &g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.Group{}, groups.ErrNotFound
		}
		return groups.Group{}, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

func (r *Repository) ListForUser(ctx context.Context, userID string) ([]groups.Group, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT g.id, g.app_id, g.name, g.status, g.metadata, g.created_at, g.updated_at
		 FROM groups g JOIN group_members m ON m.group_id = g.id
		 WHERE m.user_id = $1 ORDER BY g.created_at`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list groups for user: %w", err)
	}
	defer rows.Close()

	var list []groups.Group
	for rows.Next() {
		var g groups.Group
		if err := rows.Scan(&g.ID, &g.AppID, &g.Name, &g.Status, &g.Metadata, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		list = append(list, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return list, nil
}

func (r *Repository) Update(ctx context.Context, id string, in groups.UpdateInput) (groups.Group, error) {
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
	if in.Metadata != nil {
		metadata, err := marshalMetadata(in.Metadata)
		if err != nil {
			return groups.Group{}, err
		}
		args = append(args, metadata)
		setClauses += fmt.Sprintf(", metadata = $%d", len(args))
	}
	args = append(args, id)
	query := fmt.Sprintf(`UPDATE groups SET %s WHERE id = $%d RETURNING `+groupColumns, setClauses, len(args))

	var g groups.Group
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&g.ID, &g.AppID, &g.Name, &g.Status, &g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.Group{}, groups.ErrNotFound
		}
		return groups.Group{}, fmt.Errorf("update group: %w", err)
	}
	return g, nil
}

func (r *Repository) GetMembership(ctx context.Context, groupID, userID string) (groups.GroupMember, error) {
	var m groups.GroupMember
	err := r.pool.QueryRow(ctx,
		`SELECT `+memberColumns+` FROM group_members WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	).Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.IsManager, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.GroupMember{}, groups.ErrMembershipNotFound
		}
		return groups.GroupMember{}, fmt.Errorf("get membership: %w", err)
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, groupID string) ([]groups.GroupMember, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+memberColumns+` FROM group_members WHERE group_id = $1 ORDER BY created_at`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var list []groups.GroupMember
	for rows.Next() {
		var m groups.GroupMember
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.IsManager, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate members: %w", err)
	}
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, groupID string, in groups.AddMemberInput) (groups.GroupMember, error) {
	role := in.Role
	if role == "" {
		role = "member"
	}
	var m groups.GroupMember
	err := r.pool.QueryRow(ctx,
		`INSERT INTO group_members (group_id, user_id, role, is_manager) VALUES ($1, $2, $3, $4) RETURNING `+memberColumns,
		groupID, in.UserID, role, in.IsManager,
	).Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.IsManager, &m.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return groups.GroupMember{}, groups.ErrAlreadyMember
		}
		return groups.GroupMember{}, fmt.Errorf("add member: %w", err)
	}
	return m, nil
}

func (r *Repository) UpdateMember(ctx context.Context, groupID, userID string, in groups.UpdateMemberInput) (groups.GroupMember, error) {
	setClauses := []string{}
	args := []any{}
	if in.Role != nil {
		args = append(args, *in.Role)
		setClauses = append(setClauses, fmt.Sprintf("role = $%d", len(args)))
	}
	if in.IsManager != nil {
		args = append(args, *in.IsManager)
		setClauses = append(setClauses, fmt.Sprintf("is_manager = $%d", len(args)))
	}
	args = append(args, groupID, userID)
	query := fmt.Sprintf(`UPDATE group_members SET %s WHERE group_id = $%d AND user_id = $%d RETURNING `+memberColumns,
		strings.Join(setClauses, ", "), len(args)-1, len(args))

	var m groups.GroupMember
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.IsManager, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.GroupMember{}, groups.ErrMembershipNotFound
		}
		return groups.GroupMember{}, fmt.Errorf("update member: %w", err)
	}
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, groupID, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return groups.ErrMembershipNotFound
	}
	return nil
}

func marshalMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
