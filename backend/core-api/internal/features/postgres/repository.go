// Package postgres is the PostgreSQL-backed features.Repository.
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

	"github.com/example/core-platform/backend/core-api/internal/features"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const featureColumns = "id, app_id, key, name, description, enabled, created_at, updated_at"
const ruleColumns = "id, feature_id, priority, conditions, enabled, created_at, updated_at"

func scanFeature(row pgx.Row) (features.Feature, error) {
	var f features.Feature
	err := row.Scan(&f.ID, &f.AppID, &f.Key, &f.Name, &f.Description, &f.Enabled, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func scanRule(row pgx.Row) (features.FeatureRule, error) {
	var r features.FeatureRule
	err := row.Scan(&r.ID, &r.FeatureID, &r.Priority, &r.Conditions, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *Repository) CreateFeature(ctx context.Context, in features.CreateFeatureInput) (features.Feature, error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	f, err := scanFeature(r.pool.QueryRow(ctx,
		`INSERT INTO features (app_id, key, name, description, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING `+featureColumns,
		in.AppID, in.Key, in.Name, in.Description, enabled,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return features.Feature{}, features.ErrKeyTaken
		}
		return features.Feature{}, fmt.Errorf("insert feature: %w", err)
	}
	return f, nil
}

func (r *Repository) GetFeature(ctx context.Context, appID, key string) (features.Feature, error) {
	f, err := scanFeature(r.pool.QueryRow(ctx,
		`SELECT `+featureColumns+` FROM features WHERE app_id = $1 AND key = $2`, appID, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return features.Feature{}, features.ErrNotFound
		}
		return features.Feature{}, fmt.Errorf("get feature: %w", err)
	}
	return f, nil
}

func (r *Repository) ListFeatures(ctx context.Context, appID string) ([]features.Feature, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+featureColumns+` FROM features WHERE app_id = $1 ORDER BY key`, appID)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()

	var list []features.Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		list = append(list, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate features: %w", err)
	}
	return list, nil
}

func (r *Repository) UpdateFeature(ctx context.Context, appID, key string, in features.UpdateFeatureInput) (features.Feature, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	if in.Name != nil {
		args = append(args, *in.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if in.Description != nil {
		args = append(args, *in.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", len(args)))
	}
	if in.Enabled != nil {
		args = append(args, *in.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}
	args = append(args, appID, key)
	query := fmt.Sprintf(`UPDATE features SET %s WHERE app_id = $%d AND key = $%d RETURNING `+featureColumns,
		strings.Join(setClauses, ", "), len(args)-1, len(args))

	f, err := scanFeature(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return features.Feature{}, features.ErrNotFound
		}
		return features.Feature{}, fmt.Errorf("update feature: %w", err)
	}
	return f, nil
}

func (r *Repository) CreateRule(ctx context.Context, featureID string, in features.CreateRuleInput) (features.FeatureRule, error) {
	conditions, err := json.Marshal(in.Conditions)
	if err != nil {
		return features.FeatureRule{}, fmt.Errorf("marshal conditions: %w", err)
	}
	rule, err := scanRule(r.pool.QueryRow(ctx,
		`INSERT INTO feature_rules (feature_id, priority, conditions, enabled) VALUES ($1, $2, $3, $4) RETURNING `+ruleColumns,
		featureID, in.Priority, conditions, in.Enabled,
	))
	if err != nil {
		return features.FeatureRule{}, fmt.Errorf("insert rule: %w", err)
	}
	return rule, nil
}

func (r *Repository) ListRules(ctx context.Context, featureID string) ([]features.FeatureRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+ruleColumns+` FROM feature_rules WHERE feature_id = $1 ORDER BY priority`, featureID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var list []features.FeatureRule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		list = append(list, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return list, nil
}

func (r *Repository) GetRule(ctx context.Context, ruleID string) (features.FeatureRule, error) {
	rule, err := scanRule(r.pool.QueryRow(ctx, `SELECT `+ruleColumns+` FROM feature_rules WHERE id = $1`, ruleID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return features.FeatureRule{}, features.ErrRuleNotFound
		}
		return features.FeatureRule{}, fmt.Errorf("get rule: %w", err)
	}
	return rule, nil
}

func (r *Repository) UpdateRule(ctx context.Context, ruleID string, in features.UpdateRuleInput) (features.FeatureRule, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	if in.Priority != nil {
		args = append(args, *in.Priority)
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", len(args)))
	}
	if in.Conditions != nil {
		conditions, err := json.Marshal(*in.Conditions)
		if err != nil {
			return features.FeatureRule{}, fmt.Errorf("marshal conditions: %w", err)
		}
		args = append(args, conditions)
		setClauses = append(setClauses, fmt.Sprintf("conditions = $%d", len(args)))
	}
	if in.Enabled != nil {
		args = append(args, *in.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}
	args = append(args, ruleID)
	query := fmt.Sprintf(`UPDATE feature_rules SET %s WHERE id = $%d RETURNING `+ruleColumns,
		strings.Join(setClauses, ", "), len(args))

	rule, err := scanRule(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return features.FeatureRule{}, features.ErrRuleNotFound
		}
		return features.FeatureRule{}, fmt.Errorf("update rule: %w", err)
	}
	return rule, nil
}

func (r *Repository) DeleteRule(ctx context.Context, ruleID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM feature_rules WHERE id = $1`, ruleID)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return features.ErrRuleNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
