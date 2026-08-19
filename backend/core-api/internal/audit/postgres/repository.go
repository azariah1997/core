// Package postgres is the PostgreSQL-backed audit.Repository, built on
// the pre-existing scaffold "audit_events" table (extended by
// 0017_audit.sql with tenant_id/device_id, plus a database-level
// immutability trigger). Notably absent: any Update or Delete method -
// this Repository cannot violate "audit must be immutable" even if a
// caller wanted it to.
package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/audit"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = "id, app_id, actor_user_id, action, resource_type, resource_id, tenant_id, device_id, correlation_id, metadata, occurred_at"

func scanRecord(row pgx.Row) (audit.Record, error) {
	var r audit.Record
	var appID, actorUserID, tenantID, deviceID, correlationID *string
	err := row.Scan(&r.ID, &appID, &actorUserID, &r.Action, &r.ResourceType, &r.ResourceID,
		&tenantID, &deviceID, &correlationID, &r.Metadata, &r.OccurredAt)
	if err != nil {
		return audit.Record{}, err
	}
	if appID != nil {
		r.AppID = *appID
	}
	if actorUserID != nil {
		r.ActorUserID = *actorUserID
	}
	if tenantID != nil {
		r.TenantID = *tenantID
	}
	if deviceID != nil {
		r.DeviceID = *deviceID
	}
	if correlationID != nil {
		r.CorrelationID = *correlationID
	}
	return r, nil
}

func (r *Repository) Record(ctx context.Context, in audit.RecordInput, correlationID string) (audit.Record, error) {
	metadata, err := marshalMetadata(in.Metadata)
	if err != nil {
		return audit.Record{}, err
	}
	rec, err := scanRecord(r.pool.QueryRow(ctx,
		`INSERT INTO audit_events (app_id, actor_user_id, action, resource_type, resource_id, tenant_id, device_id, correlation_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+columns,
		nullIfEmpty(in.AppID), nullIfEmpty(in.ActorUserID), in.Action, in.ResourceType, in.ResourceID,
		nullIfEmpty(in.TenantID), nullIfEmpty(in.DeviceID), nullIfEmpty(correlationID), metadata,
	))
	if err != nil {
		return audit.Record{}, fmt.Errorf("insert audit record: %w", err)
	}
	return rec, nil
}

func (r *Repository) Get(ctx context.Context, id string) (audit.Record, error) {
	rec, err := scanRecord(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM audit_events WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return audit.Record{}, audit.ErrNotFound
		}
		return audit.Record{}, fmt.Errorf("get audit record: %w", err)
	}
	return rec, nil
}

func (r *Repository) List(ctx context.Context, filter audit.ListFilter) (audit.ListResult, error) {
	where := []string{"1=1"}
	args := []any{}
	addFilter := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		where = append(where, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	addFilter("app_id", filter.AppID)
	addFilter("actor_user_id", filter.ActorUserID)
	addFilter("resource_type", filter.ResourceType)
	addFilter("resource_id", filter.ResourceID)
	addFilter("action", filter.Action)

	if filter.Cursor != "" {
		beforeOccurred, beforeID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return audit.ListResult{}, &audit.ValidationError{Message: "invalid cursor"}
		}
		args = append(args, beforeOccurred, beforeID)
		where = append(where, fmt.Sprintf("(occurred_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, filter.Limit+1)
	query := `SELECT ` + columns + ` FROM audit_events WHERE ` + strings.Join(where, " AND ") +
		fmt.Sprintf(` ORDER BY occurred_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return audit.ListResult{}, fmt.Errorf("list audit records: %w", err)
	}
	defer rows.Close()

	var items []audit.Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return audit.ListResult{}, fmt.Errorf("scan audit record: %w", err)
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return audit.ListResult{}, fmt.Errorf("iterate audit records: %w", err)
	}

	result := audit.ListResult{Items: items}
	if len(items) > filter.Limit {
		last := items[filter.Limit-1]
		result.Items = items[:filter.Limit]
		result.NextCursor = encodeCursor(last.OccurredAt, last.ID)
	}
	return result, nil
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}
