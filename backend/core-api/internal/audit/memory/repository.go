// Package memory is an in-memory audit.Repository for tests. Like the
// postgres implementation, it has no Update or Delete method.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/audit"
)

type Repository struct {
	mu      sync.Mutex
	records map[string]audit.Record
}

func New() *Repository {
	return &Repository{records: map[string]audit.Record{}}
}

func (r *Repository) Record(ctx context.Context, in audit.RecordInput, correlationID string) (audit.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := audit.Record{
		ID: uuid.NewString(), ActorUserID: in.ActorUserID, Action: in.Action, ResourceType: in.ResourceType,
		ResourceID: in.ResourceID, AppID: in.AppID, TenantID: in.TenantID, DeviceID: in.DeviceID,
		CorrelationID: correlationID, Metadata: in.Metadata, OccurredAt: time.Now().UTC(),
	}
	r.records[rec.ID] = rec
	return rec, nil
}

func (r *Repository) Get(ctx context.Context, id string) (audit.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return audit.Record{}, audit.ErrNotFound
	}
	return rec, nil
}

func (r *Repository) List(ctx context.Context, filter audit.ListFilter) (audit.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []audit.Record
	for _, rec := range r.records {
		if filter.AppID != "" && rec.AppID != filter.AppID {
			continue
		}
		if filter.ActorUserID != "" && rec.ActorUserID != filter.ActorUserID {
			continue
		}
		if filter.ResourceType != "" && rec.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && rec.ResourceID != filter.ResourceID {
			continue
		}
		if filter.Action != "" && rec.Action != filter.Action {
			continue
		}
		all = append(all, rec)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].OccurredAt.Equal(all[j].OccurredAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].OccurredAt.After(all[j].OccurredAt)
	})

	start := 0
	if filter.Cursor != "" {
		afterID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return audit.ListResult{}, &audit.ValidationError{Message: "invalid cursor"}
		}
		for i, rec := range all {
			if rec.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + filter.Limit
	if end > len(all) {
		end = len(all)
	}
	if start > len(all) {
		start = len(all)
	}
	page := append([]audit.Record{}, all[start:end]...)

	result := audit.ListResult{Items: page}
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
