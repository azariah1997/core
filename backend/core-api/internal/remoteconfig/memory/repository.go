// Package memory is an in-memory remoteconfig.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
)

type Repository struct {
	mu      sync.Mutex
	entries map[string]remoteconfig.Entry // scopeKey -> entry
	history map[string][]remoteconfig.Change
}

func New() *Repository {
	return &Repository{entries: map[string]remoteconfig.Entry{}, history: map[string][]remoteconfig.Change{}}
}

func scopeKey(appID, environment, key string) string { return appID + "|" + environment + "|" + key }

func (r *Repository) Set(ctx context.Context, changedBy string, in remoteconfig.SetInput) (remoteconfig.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := scopeKey(in.AppID, in.Environment, in.Key)
	now := time.Now().UTC()

	existing, hadPrevious := r.entries[k]
	var previousValue any
	if hadPrevious {
		previousValue = existing.Value
	}

	entry := remoteconfig.Entry{
		ID: k, AppID: in.AppID, Environment: in.Environment, Key: in.Key, Value: in.Value,
		Description: in.Description, UpdatedBy: changedBy, UpdatedAt: now,
	}
	if hadPrevious {
		entry.ID = existing.ID
		entry.CreatedAt = existing.CreatedAt
		if entry.Description == "" {
			entry.Description = existing.Description
		}
	} else {
		entry.ID = uuid.NewString()
		entry.CreatedAt = now
	}
	r.entries[k] = entry

	r.history[k] = append(r.history[k], remoteconfig.Change{
		ID: uuid.NewString(), AppID: in.AppID, Environment: in.Environment, Key: in.Key,
		PreviousValue: previousValue, NewValue: in.Value, ChangedBy: changedBy, Reason: in.Reason, ChangedAt: now,
	})
	return entry, nil
}

func (r *Repository) Get(ctx context.Context, appID, environment, key string) (remoteconfig.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[scopeKey(appID, environment, key)]
	if !ok {
		return remoteconfig.Entry{}, remoteconfig.ErrNotFound
	}
	return e, nil
}

func (r *Repository) List(ctx context.Context, appID, environment string) ([]remoteconfig.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []remoteconfig.Entry
	for _, e := range r.entries {
		if e.AppID == appID && e.Environment == environment {
			list = append(list, e)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	return list, nil
}

func (r *Repository) Delete(ctx context.Context, changedBy, appID, environment, key, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := scopeKey(appID, environment, key)
	existing, ok := r.entries[k]
	if !ok {
		return remoteconfig.ErrNotFound
	}
	delete(r.entries, k)
	r.history[k] = append(r.history[k], remoteconfig.Change{
		ID: uuid.NewString(), AppID: appID, Environment: environment, Key: key,
		PreviousValue: existing.Value, NewValue: nil, ChangedBy: changedBy, Reason: reason, ChangedAt: time.Now().UTC(),
	})
	return nil
}

func (r *Repository) History(ctx context.Context, appID, environment, key string) ([]remoteconfig.Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changes := r.history[scopeKey(appID, environment, key)]
	// Newest first, matching the postgres implementation's ORDER BY changed_at DESC.
	out := make([]remoteconfig.Change, len(changes))
	for i, c := range changes {
		out[len(changes)-1-i] = c
	}
	return out, nil
}
