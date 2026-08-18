// Package memory is an in-memory jobs.Repository for tests.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/jobs"
)

type Repository struct {
	mu   sync.Mutex
	jobs map[string]jobs.Job
}

func New() *Repository {
	return &Repository{jobs: map[string]jobs.Job{}}
}

func (r *Repository) Create(ctx context.Context, createdBy string, in jobs.EnqueueInput, runAt time.Time, maxAttempts int, recurrence *time.Duration) (jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	j := jobs.Job{
		ID: uuid.NewString(), AppID: in.AppID, Type: in.Type, Payload: in.Payload,
		Status: jobs.StatusScheduled, RunAt: runAt, RecurrenceInterval: recurrence,
		MaxAttempts: maxAttempts, CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now,
	}
	r.jobs[j.ID] = j
	return j, nil
}

func (r *Repository) Get(ctx context.Context, id string) (jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return j, nil
}

func (r *Repository) ListForCaller(ctx context.Context, callerID string, params jobs.ListParams) (jobs.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []jobs.Job
	for _, j := range r.jobs {
		if j.CreatedBy == callerID {
			all = append(all, j)
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
			return jobs.ListResult{}, &jobs.ValidationError{Message: "invalid cursor"}
		}
		for i, j := range all {
			if j.ID == afterID {
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
	page := append([]jobs.Job{}, all[start:end]...)

	result := jobs.ListResult{Items: page}
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

// ListAttempts always returns empty - executing (and recording attempts
// for) a job is worker's job, and this in-memory repository never runs
// in the worker process; core-api's own unit tests only exercise the
// enqueue/query surface.
func (r *Repository) ListAttempts(ctx context.Context, jobID string) ([]jobs.JobAttempt, error) {
	return nil, nil
}
