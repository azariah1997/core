// Package postgres is the PostgreSQL-backed files.Repository, built on
// the pre-existing scaffold "files" table (extended by 0012_files.sql
// with file_name/status/checksum/expires_at/updated_at) - same "adapt to
// what's already there" pattern as every prior phase.
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

	"github.com/example/core-platform/backend/core-api/internal/files"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = "id, app_id, owner_user_id, object_key, file_name, mime_type, byte_size, checksum, visibility, status, expires_at, created_at, updated_at"

func (r *Repository) Create(ctx context.Context, ownerUserID, objectKey string, in files.RequestUploadInput) (files.File, error) {
	var expiresAt *time.Time
	if in.RetentionDays != nil {
		t := time.Now().AddDate(0, 0, *in.RetentionDays)
		expiresAt = &t
	}
	var f files.File
	var checksum *string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO files (app_id, owner_user_id, object_key, file_name, mime_type, byte_size, checksum, visibility, status, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', $9) RETURNING `+columns,
		in.AppID, ownerUserID, objectKey, in.FileName, in.MimeType, in.SizeBytes, nullIfEmpty(in.Checksum), string(in.Visibility), expiresAt,
	).Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return files.File{}, fmt.Errorf("insert file: %w", err)
	}
	if checksum != nil {
		f.Checksum = *checksum
	}
	return f, nil
}

func (r *Repository) Get(ctx context.Context, id string) (files.File, error) {
	var f files.File
	var checksum *string
	err := r.pool.QueryRow(ctx, `SELECT `+columns+` FROM files WHERE id = $1`, id).
		Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, files.ErrNotFound
		}
		return files.File{}, fmt.Errorf("get file: %w", err)
	}
	if checksum != nil {
		f.Checksum = *checksum
	}
	return f, nil
}

func (r *Repository) ListForOwner(ctx context.Context, ownerUserID string, params files.ListParams) (files.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+columns+` FROM files WHERE owner_user_id = $1 AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT $2`,
			ownerUserID, params.Limit+1)
	} else {
		beforeCreated, beforeID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return files.ListResult{}, &files.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+columns+` FROM files
			 WHERE owner_user_id = $1 AND status != 'deleted' AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			ownerUserID, beforeCreated, beforeID, params.Limit+1)
	}
	if err != nil {
		return files.ListResult{}, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var items []files.File
	for rows.Next() {
		var f files.File
		var checksum *string
		if err := rows.Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return files.ListResult{}, fmt.Errorf("scan file: %w", err)
		}
		if checksum != nil {
			f.Checksum = *checksum
		}
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return files.ListResult{}, fmt.Errorf("iterate files: %w", err)
	}

	result := files.ListResult{Items: items}
	if len(items) > params.Limit {
		last := items[params.Limit-1]
		result.Items = items[:params.Limit]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return result, nil
}

// ConfirmUpload writes the real size/checksum storage reported (never
// the client's unverified claim) and the file.uploaded outbox event in
// one transaction.
func (r *Repository) ConfirmUpload(ctx context.Context, id string, actualSize int64, actualChecksum string) (files.File, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return files.File{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var f files.File
	var checksum *string
	err = tx.QueryRow(ctx,
		`UPDATE files SET status = 'active', byte_size = $1, checksum = $2, updated_at = now() WHERE id = $3 RETURNING `+columns,
		actualSize, actualChecksum, id,
	).Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, files.ErrNotFound
		}
		return files.File{}, fmt.Errorf("confirm upload: %w", err)
	}
	if checksum != nil {
		f.Checksum = *checksum
	}

	payload, err := json.Marshal(map[string]any{
		"id": f.ID, "appId": f.AppID, "ownerUserId": f.OwnerUserID, "mimeType": f.MimeType, "byteSize": f.ByteSize,
	})
	if err != nil {
		return files.File{}, fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('file', $1, 'file.uploaded', 1, $2, $3)`,
		f.ID, payload, nullIfEmpty(correlation.FromContext(ctx)),
	)
	if err != nil {
		return files.File{}, fmt.Errorf("insert outbox event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return files.File{}, fmt.Errorf("commit tx: %w", err)
	}
	return f, nil
}

func (r *Repository) SoftDelete(ctx context.Context, id string) (files.File, error) {
	var f files.File
	var checksum *string
	err := r.pool.QueryRow(ctx,
		`UPDATE files SET status = 'deleted', updated_at = now() WHERE id = $1 RETURNING `+columns, id,
	).Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, files.ErrNotFound
		}
		return files.File{}, fmt.Errorf("soft delete file: %w", err)
	}
	if checksum != nil {
		f.Checksum = *checksum
	}
	return f, nil
}

func (r *Repository) ListExpired(ctx context.Context, before time.Time) ([]files.File, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columns+` FROM files WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at < $1`, before)
	if err != nil {
		return nil, fmt.Errorf("list expired files: %w", err)
	}
	defer rows.Close()

	var list []files.File
	for rows.Next() {
		var f files.File
		var checksum *string
		if err := rows.Scan(&f.ID, &f.AppID, &f.OwnerUserID, &f.ObjectKey, &f.FileName, &f.MimeType, &f.ByteSize, &checksum, &f.Visibility, &f.Status, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		if checksum != nil {
			f.Checksum = *checksum
		}
		list = append(list, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired files: %w", err)
	}
	return list, nil
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
