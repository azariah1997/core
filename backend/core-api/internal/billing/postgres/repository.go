// Package postgres is the PostgreSQL-backed billing.Repository.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/billing"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const entitlementColumns = `id, user_id, key, source, granted_at, expires_at, revoked_at`

func scanEntitlement(row interface{ Scan(...any) error }) (billing.Entitlement, error) {
	var e billing.Entitlement
	err := row.Scan(&e.ID, &e.UserID, &e.Key, &e.Source, &e.GrantedAt, &e.ExpiresAt, &e.RevokedAt)
	return e, err
}

func (r *Repository) GrantEntitlement(ctx context.Context, e billing.Entitlement) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO entitlements (id, user_id, key, source, granted_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.UserID, e.Key, e.Source, e.GrantedAt, e.ExpiresAt)
	if err != nil {
		return fmt.Errorf("grant entitlement: %w", err)
	}
	return nil
}

func (r *Repository) RevokeEntitlement(ctx context.Context, id string) (billing.Entitlement, error) {
	e, err := scanEntitlement(r.pool.QueryRow(ctx,
		`UPDATE entitlements SET revoked_at = now() WHERE id = $1 RETURNING `+entitlementColumns, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return billing.Entitlement{}, billing.ErrNotFound
		}
		return billing.Entitlement{}, fmt.Errorf("revoke entitlement: %w", err)
	}
	return e, nil
}

// RevokeBySource is idempotent by construction - the WHERE clause only
// ever matches still-active rows, so revoking the same source twice
// (a redelivered subscription.deleted webhook) affects zero rows the
// second time rather than erroring.
func (r *Repository) RevokeBySource(ctx context.Context, source string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE entitlements SET revoked_at = now() WHERE source = $1 AND revoked_at IS NULL`, source)
	if err != nil {
		return fmt.Errorf("revoke entitlements by source: %w", err)
	}
	return nil
}

func (r *Repository) ListEntitlements(ctx context.Context, userID string) ([]billing.Entitlement, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+entitlementColumns+` FROM entitlements WHERE user_id = $1 ORDER BY granted_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list entitlements: %w", err)
	}
	defer rows.Close()
	var list []billing.Entitlement
	for rows.Next() {
		e, err := scanEntitlement(rows)
		if err != nil {
			return nil, fmt.Errorf("scan entitlement: %w", err)
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (r *Repository) GetEntitlement(ctx context.Context, id string) (billing.Entitlement, error) {
	e, err := scanEntitlement(r.pool.QueryRow(ctx, `SELECT `+entitlementColumns+` FROM entitlements WHERE id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return billing.Entitlement{}, billing.ErrNotFound
		}
		return billing.Entitlement{}, fmt.Errorf("get entitlement: %w", err)
	}
	return e, nil
}

const paymentColumns = `id, user_id, provider, provider_ref, amount_cents, currency, status, metadata, created_at, updated_at`

func scanPayment(row pgx.Row) (billing.Payment, error) {
	var p billing.Payment
	var metadata []byte
	if err := row.Scan(&p.ID, &p.UserID, &p.Provider, &p.ProviderRef, &p.AmountCents, &p.Currency, &p.Status, &metadata, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return billing.Payment{}, err
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &p.Metadata); err != nil {
			return billing.Payment{}, fmt.Errorf("unmarshal payment metadata: %w", err)
		}
	}
	return p, nil
}

// RecordPayment relies on the migration's UNIQUE (provider, provider_ref)
// constraint: ON CONFLICT DO NOTHING means a redelivered webhook's
// INSERT is skipped, and RETURNING then yields zero rows - the
// pgx.ErrNoRows case below - which is exactly how this method tells
// Service.HandleWebhook "this was already processed, don't grant
// again" without a separate SELECT-then-INSERT race.
func (r *Repository) RecordPayment(ctx context.Context, p billing.Payment) (billing.Payment, bool, error) {
	metadata, err := json.Marshal(p.Metadata)
	if err != nil {
		return billing.Payment{}, false, fmt.Errorf("marshal payment metadata: %w", err)
	}
	row := r.pool.QueryRow(ctx,
		`INSERT INTO payments (id, user_id, provider, provider_ref, amount_cents, currency, status, metadata, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (provider, provider_ref) DO NOTHING
		 RETURNING `+paymentColumns,
		p.ID, p.UserID, p.Provider, p.ProviderRef, p.AmountCents, p.Currency, p.Status, metadata, p.CreatedAt, p.UpdatedAt)
	out, err := scanPayment(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return billing.Payment{}, false, nil
		}
		return billing.Payment{}, false, fmt.Errorf("record payment: %w", err)
	}
	return out, true, nil
}

func (r *Repository) ListPayments(ctx context.Context, userID string) ([]billing.Payment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+paymentColumns+` FROM payments WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()
	var list []billing.Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
