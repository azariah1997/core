// Package postgres is the PostgreSQL-backed relationships.Repository. It
// writes RequesterID/TargetID into the existing user_a/user_b columns
// (user_a = requester, user_b = target by this package's convention) -
// the table predates this package and its symmetric LEAST/GREATEST unique
// index doesn't care which column holds which side.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/relationships"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = "id, app_id, user_a, user_b, relationship_type, status, metadata, created_at, updated_at"

func (r *Repository) Create(ctx context.Context, in relationships.RequestInput, status relationships.Status) (relationships.Relationship, error) {
	metadata, err := marshalMetadata(in.Metadata)
	if err != nil {
		return relationships.Relationship{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return relationships.Relationship{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var rel relationships.Relationship
	err = tx.QueryRow(ctx,
		`INSERT INTO relationships (app_id, user_a, user_b, relationship_type, status, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+columns,
		in.AppID, in.RequesterID, in.TargetID, in.Type, string(status), metadata,
	).Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return relationships.Relationship{}, relationships.ErrExists
		}
		return relationships.Relationship{}, fmt.Errorf("insert relationship: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, "relationship.created", rel); err != nil {
		return relationships.Relationship{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return relationships.Relationship{}, fmt.Errorf("commit tx: %w", err)
	}
	return rel, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, eventType string, rel relationships.Relationship) error {
	payload, err := json.Marshal(map[string]any{
		"id": rel.ID, "requesterUserId": rel.RequesterID, "targetUserId": rel.TargetID,
		"type": rel.Type, "status": rel.Status,
	})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('relationship', $1, $2, 1, $3, $4)`,
		rel.ID, eventType, payload, nullIfEmpty(correlation.FromContext(ctx)),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) Get(ctx context.Context, id string) (relationships.Relationship, error) {
	var rel relationships.Relationship
	err := r.pool.QueryRow(ctx, `SELECT `+columns+` FROM relationships WHERE id = $1`, id).
		Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return relationships.Relationship{}, relationships.ErrNotFound
		}
		return relationships.Relationship{}, fmt.Errorf("get relationship: %w", err)
	}
	return rel, nil
}

func (r *Repository) FindBetween(ctx context.Context, appID, userA, userB, relType string) (relationships.Relationship, error) {
	var rel relationships.Relationship
	err := r.pool.QueryRow(ctx,
		`SELECT `+columns+` FROM relationships
		 WHERE app_id = $1 AND relationship_type = $2
		   AND ((user_a = $3 AND user_b = $4) OR (user_a = $4 AND user_b = $3))`,
		appID, relType, userA, userB,
	).Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return relationships.Relationship{}, relationships.ErrNotFound
		}
		return relationships.Relationship{}, fmt.Errorf("find relationship: %w", err)
	}
	return rel, nil
}

func (r *Repository) ListForUser(ctx context.Context, appID, userID string, filter relationships.ListFilter) ([]relationships.Relationship, error) {
	query := `SELECT ` + columns + ` FROM relationships WHERE app_id = $1 AND (user_a = $2 OR user_b = $2)`
	args := []any{appID, userID}
	if filter.Type != "" {
		args = append(args, filter.Type)
		query += fmt.Sprintf(" AND relationship_type = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list relationships: %w", err)
	}
	defer rows.Close()

	var list []relationships.Relationship
	for rows.Next() {
		var rel relationships.Relationship
		if err := rows.Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		list = append(list, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relationships: %w", err)
	}
	return list, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status relationships.Status) (relationships.Relationship, error) {
	var rel relationships.Relationship
	err := r.pool.QueryRow(ctx,
		`UPDATE relationships SET status = $1, updated_at = now() WHERE id = $2 RETURNING `+columns,
		string(status), id,
	).Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return relationships.Relationship{}, relationships.ErrNotFound
		}
		return relationships.Relationship{}, fmt.Errorf("update relationship status: %w", err)
	}
	return rel, nil
}

func (r *Repository) Revive(ctx context.Context, id string, in relationships.RequestInput) (relationships.Relationship, error) {
	metadata, err := marshalMetadata(in.Metadata)
	if err != nil {
		return relationships.Relationship{}, err
	}
	var rel relationships.Relationship
	err = r.pool.QueryRow(ctx,
		`UPDATE relationships SET user_a = $1, user_b = $2, status = 'pending', metadata = $3, updated_at = now()
		 WHERE id = $4 RETURNING `+columns,
		in.RequesterID, in.TargetID, metadata, id,
	).Scan(&rel.ID, &rel.AppID, &rel.RequesterID, &rel.TargetID, &rel.Type, &rel.Status, &rel.Metadata, &rel.CreatedAt, &rel.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return relationships.Relationship{}, relationships.ErrNotFound
		}
		return relationships.Relationship{}, fmt.Errorf("revive relationship: %w", err)
	}
	return rel, nil
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
