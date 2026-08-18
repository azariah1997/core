// Package memory is an in-memory files.Repository for tests.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/files"
)

type Repository struct {
	mu    sync.Mutex
	files map[string]files.File
}

func New() *Repository {
	return &Repository{files: map[string]files.File{}}
}

func (r *Repository) Create(ctx context.Context, ownerUserID, objectKey string, in files.RequestUploadInput) (files.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	visibility := in.Visibility
	if visibility == "" {
		visibility = files.VisibilityPrivate
	}
	var expiresAt *time.Time
	if in.RetentionDays != nil {
		t := now.AddDate(0, 0, *in.RetentionDays)
		expiresAt = &t
	}
	f := files.File{
		ID: uuid.NewString(), AppID: in.AppID, OwnerUserID: ownerUserID, ObjectKey: objectKey,
		FileName: in.FileName, MimeType: in.MimeType, ByteSize: in.SizeBytes, Checksum: in.Checksum,
		Visibility: visibility, Status: files.StatusPending, ExpiresAt: expiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	r.files[f.ID] = f
	return f, nil
}

func (r *Repository) Get(ctx context.Context, id string) (files.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return files.File{}, files.ErrNotFound
	}
	return f, nil
}

func (r *Repository) ListForOwner(ctx context.Context, ownerUserID string, params files.ListParams) (files.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []files.File
	for _, f := range r.files {
		if f.OwnerUserID == ownerUserID && f.Status != files.StatusDeleted {
			all = append(all, f)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	start := 0
	if params.Cursor != "" {
		afterID, err := decodeCursor(params.Cursor)
		if err != nil {
			return files.ListResult{}, &files.ValidationError{Message: "invalid cursor"}
		}
		for i, f := range all {
			if f.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + params.Limit
	if end > len(all) {
		end = len(all)
	}
	if start > len(all) {
		start = len(all)
	}
	page := append([]files.File{}, all[start:end]...)

	result := files.ListResult{Items: page}
	if end < len(all) {
		result.NextCursor = encodeCursor(page[len(page)-1].ID)
	}
	return result, nil
}

func encodeCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func decodeCursor(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	return string(raw), err
}

func (r *Repository) ConfirmUpload(ctx context.Context, id string, actualSize int64, actualChecksum string) (files.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return files.File{}, files.ErrNotFound
	}
	f.Status = files.StatusActive
	f.ByteSize = actualSize
	f.Checksum = actualChecksum
	f.UpdatedAt = time.Now().UTC()
	r.files[id] = f
	return f, nil
}

func (r *Repository) SoftDelete(ctx context.Context, id string) (files.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return files.File{}, files.ErrNotFound
	}
	f.Status = files.StatusDeleted
	f.UpdatedAt = time.Now().UTC()
	r.files[id] = f
	return f, nil
}

func (r *Repository) ListExpired(ctx context.Context, before time.Time) ([]files.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []files.File
	for _, f := range r.files {
		if f.Status == files.StatusActive && f.ExpiresAt != nil && f.ExpiresAt.Before(before) {
			list = append(list, f)
		}
	}
	return list, nil
}
