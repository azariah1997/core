// Package postgres is the PostgreSQL-backed aigateway.Repository. Text
// is deliberately never written or read here - see
// aigateway.Completion's own doc comment for why.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) RecordCompletion(ctx context.Context, c aigateway.Completion) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO ai_completions (id, user_id, app_id, model_alias, provider, model, prompt_key, prompt_version,
		                              prompt_tokens, completion_tokens, total_tokens, cost_cents, latency_ms, finish_reason,
		                              status, error, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		c.ID, nullIfEmpty(c.UserID), nullIfEmpty(c.AppID), c.ModelAlias, nullIfEmpty(c.Provider), nullIfEmpty(c.Model),
		nullIfEmpty(c.PromptKey), nullIfEmpty(c.PromptVersion), c.PromptTokens, c.CompletionTokens, c.TotalTokens,
		c.CostCents, c.LatencyMS, nullIfEmpty(c.FinishReason), c.Status, nullIfEmpty(c.Error), c.CreatedAt)
	if err != nil {
		return fmt.Errorf("record completion: %w", err)
	}
	return nil
}

const completionColumns = `id, coalesce(user_id::text, ''), coalesce(app_id::text, ''), model_alias, coalesce(provider, ''),
	coalesce(model, ''), coalesce(prompt_key, ''), coalesce(prompt_version, ''), prompt_tokens, completion_tokens,
	total_tokens, cost_cents, latency_ms, coalesce(finish_reason, ''), status, coalesce(error, ''), created_at`

func (r *Repository) ListCompletions(ctx context.Context, userID string, limit int) ([]aigateway.Completion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+completionColumns+` FROM ai_completions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list completions: %w", err)
	}
	defer rows.Close()

	var list []aigateway.Completion
	for rows.Next() {
		var c aigateway.Completion
		if err := rows.Scan(&c.ID, &c.UserID, &c.AppID, &c.ModelAlias, &c.Provider, &c.Model, &c.PromptKey, &c.PromptVersion,
			&c.PromptTokens, &c.CompletionTokens, &c.TotalTokens, &c.CostCents, &c.LatencyMS, &c.FinishReason,
			&c.Status, &c.Error, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan completion: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
