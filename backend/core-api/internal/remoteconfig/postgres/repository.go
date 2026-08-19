// Package postgres is the PostgreSQL-backed remoteconfig.Repository.
// Every write (Set or Delete) records its Entry change and emits a
// config.updated/config.deleted outbox event in the same transaction -
// the audit trail "all changes must be auditable" requires, plus a seam
// a future central Audit Service (Phase 19) could consume via the same
// outbox-polling pattern Phase 14's search indexer already established,
// without this package needing to know that service exists.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const entryColumns = "id, app_id, environment, key, value, description, updated_by, created_at, updated_at"

func scanEntry(row pgx.Row) (remoteconfig.Entry, error) {
	var e remoteconfig.Entry
	var updatedBy *string
	err := row.Scan(&e.ID, &e.AppID, &e.Environment, &e.Key, &e.Value, &e.Description, &updatedBy, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return remoteconfig.Entry{}, err
	}
	if updatedBy != nil {
		e.UpdatedBy = *updatedBy
	}
	return e, nil
}

func (r *Repository) Set(ctx context.Context, changedBy string, in remoteconfig.SetInput) (remoteconfig.Entry, error) {
	value, err := json.Marshal(in.Value)
	if err != nil {
		return remoteconfig.Entry{}, fmt.Errorf("marshal value: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return remoteconfig.Entry{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	previous, err := scanEntry(tx.QueryRow(ctx,
		`SELECT `+entryColumns+` FROM config_entries WHERE app_id = $1 AND environment = $2 AND key = $3`,
		in.AppID, in.Environment, in.Key))
	var previousValue any
	hadPrevious := true
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return remoteconfig.Entry{}, fmt.Errorf("check existing entry: %w", err)
		}
		hadPrevious = false
	} else {
		previousValue = previous.Value
	}

	entry, err := scanEntry(tx.QueryRow(ctx,
		`INSERT INTO config_entries (app_id, environment, key, value, description, updated_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (app_id, environment, key) DO UPDATE SET
		   value = $4, description = CASE WHEN $5 = '' THEN config_entries.description ELSE $5 END,
		   updated_by = $6, updated_at = now()
		 RETURNING `+entryColumns,
		in.AppID, in.Environment, in.Key, value, in.Description, nullIfEmpty(changedBy),
	))
	if err != nil {
		return remoteconfig.Entry{}, fmt.Errorf("upsert config entry: %w", err)
	}

	if err := recordChange(ctx, tx, changeInput{
		appID: in.AppID, environment: in.Environment, key: in.Key,
		previousValue: previousValue, hadPrevious: hadPrevious, newValue: entry.Value, hasNewValue: true,
		changedBy: changedBy, reason: in.Reason,
	}); err != nil {
		return remoteconfig.Entry{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return remoteconfig.Entry{}, fmt.Errorf("commit tx: %w", err)
	}
	return entry, nil
}

func (r *Repository) Get(ctx context.Context, appID, environment, key string) (remoteconfig.Entry, error) {
	entry, err := scanEntry(r.pool.QueryRow(ctx,
		`SELECT `+entryColumns+` FROM config_entries WHERE app_id = $1 AND environment = $2 AND key = $3`,
		appID, environment, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return remoteconfig.Entry{}, remoteconfig.ErrNotFound
		}
		return remoteconfig.Entry{}, fmt.Errorf("get config entry: %w", err)
	}
	return entry, nil
}

func (r *Repository) List(ctx context.Context, appID, environment string) ([]remoteconfig.Entry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+entryColumns+` FROM config_entries WHERE app_id = $1 AND environment = $2 ORDER BY key`,
		appID, environment)
	if err != nil {
		return nil, fmt.Errorf("list config entries: %w", err)
	}
	defer rows.Close()

	var list []remoteconfig.Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan config entry: %w", err)
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config entries: %w", err)
	}
	return list, nil
}

func (r *Repository) Delete(ctx context.Context, changedBy, appID, environment, key, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, err := scanEntry(tx.QueryRow(ctx,
		`SELECT `+entryColumns+` FROM config_entries WHERE app_id = $1 AND environment = $2 AND key = $3`,
		appID, environment, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return remoteconfig.ErrNotFound
		}
		return fmt.Errorf("get config entry: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM config_entries WHERE app_id = $1 AND environment = $2 AND key = $3`, appID, environment, key,
	); err != nil {
		return fmt.Errorf("delete config entry: %w", err)
	}

	if err := recordChange(ctx, tx, changeInput{
		appID: appID, environment: environment, key: key,
		previousValue: existing.Value, hadPrevious: true, hasNewValue: false,
		changedBy: changedBy, reason: reason,
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *Repository) History(ctx context.Context, appID, environment, key string) ([]remoteconfig.Change, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, app_id, environment, key, previous_value, new_value, changed_by, reason, changed_at
		 FROM config_changes WHERE app_id = $1 AND environment = $2 AND key = $3 ORDER BY changed_at DESC`,
		appID, environment, key)
	if err != nil {
		return nil, fmt.Errorf("list config changes: %w", err)
	}
	defer rows.Close()

	var list []remoteconfig.Change
	for rows.Next() {
		var c remoteconfig.Change
		var changedBy *string
		if err := rows.Scan(&c.ID, &c.AppID, &c.Environment, &c.Key, &c.PreviousValue, &c.NewValue, &changedBy, &c.Reason, &c.ChangedAt); err != nil {
			return nil, fmt.Errorf("scan config change: %w", err)
		}
		if changedBy != nil {
			c.ChangedBy = *changedBy
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config changes: %w", err)
	}
	return list, nil
}

type changeInput struct {
	appID, environment, key string
	previousValue           any
	hadPrevious             bool
	newValue                any
	hasNewValue             bool
	changedBy, reason       string
}

func recordChange(ctx context.Context, tx pgx.Tx, in changeInput) error {
	var previousJSON, newJSON []byte
	var err error
	if in.hadPrevious {
		previousJSON, err = json.Marshal(in.previousValue)
		if err != nil {
			return fmt.Errorf("marshal previous value: %w", err)
		}
	}
	if in.hasNewValue {
		newJSON, err = json.Marshal(in.newValue)
		if err != nil {
			return fmt.Errorf("marshal new value: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO config_changes (app_id, environment, key, previous_value, new_value, changed_by, reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		in.appID, in.environment, in.key, nullIfEmptyBytes(previousJSON), nullIfEmptyBytes(newJSON), nullIfEmpty(in.changedBy), in.reason,
	); err != nil {
		return fmt.Errorf("insert config change: %w", err)
	}

	eventType := "config.updated"
	if !in.hasNewValue {
		eventType = "config.deleted"
	}
	payload, err := json.Marshal(map[string]any{
		"appId": in.appID, "environment": in.environment, "key": in.key, "changedBy": nullIfEmpty(in.changedBy),
	})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('config', $1, $2, 1, $3, $4)`,
		in.appID+"/"+in.environment+"/"+in.key, eventType, payload, nullIfEmpty(correlation.FromContext(ctx)),
	); err != nil {
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

func nullIfEmptyBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
