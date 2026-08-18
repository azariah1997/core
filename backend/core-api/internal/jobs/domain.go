// Package jobs is the platform's generic background job queue: enqueue
// here (fast - a single insert), execute in the worker process (see
// backend/worker/internal/jobrunner) - "do not execute heavy jobs inside
// HTTP request handlers" is the roadmap's own instruction, and this
// package's HTTP surface never runs a handler itself, only ever writes a
// row.
package jobs

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Status covers a Job's lifecycle. There is no "failed" status for the
// Job itself - a failed attempt that still has retries left goes back to
// Scheduled with a later RunAt; only exhausting every attempt produces a
// terminal state (DeadLetter), matching the roadmap's own retry/
// dead-letter framing. Individual attempt outcomes live on JobAttempt.
type Status string

const (
	StatusScheduled  Status = "scheduled"
	StatusRunning    Status = "running"
	StatusSucceeded  Status = "succeeded"
	StatusDeadLetter Status = "dead_letter"
)

type Job struct {
	ID                 string
	AppID              string // optional - empty for platform-global jobs
	Type               string // product/platform-defined, free-form
	Payload            map[string]any
	Status             Status
	RunAt              time.Time
	RecurrenceInterval *time.Duration // nil = one-shot
	MaxAttempts        int
	Attempts           int
	CreatedBy          string // optional - empty for system-enqueued jobs
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type JobAttempt struct {
	ID            string
	JobID         string
	AttemptNumber int
	Status        string // succeeded | failed
	Error         string
	StartedAt     time.Time
	FinishedAt    time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("job not found")
	ErrForbidden = errors.New("not permitted to access this job")
)

const defaultMaxAttempts = 5

// EnqueueInput covers all four of the roadmap's scheduling capabilities
// through one RunAt field: immediate (RunAt left zero, defaults to now),
// scheduled (RunAt set to a specific future time), and delayed
// (DelaySeconds, sugar for "now + this many seconds" - mutually
// exclusive with RunAt). Recurring is RecurrenceIntervalSeconds.
type EnqueueInput struct {
	AppID                     string
	Type                      string
	Payload                   map[string]any
	RunAt                     *time.Time
	DelaySeconds              *int
	RecurrenceIntervalSeconds *int
	MaxAttempts               *int
}

func (in EnqueueInput) Validate() error {
	switch {
	case strings.TrimSpace(in.Type) == "":
		return &ValidationError{"type is required"}
	case in.RunAt != nil && in.DelaySeconds != nil:
		return &ValidationError{"runAt and delaySeconds are mutually exclusive"}
	case in.DelaySeconds != nil && *in.DelaySeconds < 0:
		return &ValidationError{"delaySeconds must not be negative"}
	case in.RecurrenceIntervalSeconds != nil && *in.RecurrenceIntervalSeconds <= 0:
		return &ValidationError{"recurrenceIntervalSeconds must be positive"}
	case in.MaxAttempts != nil && *in.MaxAttempts <= 0:
		return &ValidationError{"maxAttempts must be positive"}
	}
	return nil
}

func (in EnqueueInput) resolveRunAt(now time.Time) time.Time {
	switch {
	case in.RunAt != nil:
		return *in.RunAt
	case in.DelaySeconds != nil:
		return now.Add(time.Duration(*in.DelaySeconds) * time.Second)
	default:
		return now
	}
}

func (in EnqueueInput) resolveMaxAttempts() int {
	if in.MaxAttempts != nil {
		return *in.MaxAttempts
	}
	return defaultMaxAttempts
}

func (in EnqueueInput) resolveRecurrenceInterval() *time.Duration {
	if in.RecurrenceIntervalSeconds == nil {
		return nil
	}
	d := time.Duration(*in.RecurrenceIntervalSeconds) * time.Second
	return &d
}

type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []Job
	NextCursor string
}

// Repository is the storage-agnostic boundary. Job creation is a single
// insert - no outbox event, unlike every write-path Repository so far:
// a job's own execution (recorded via JobAttempt) already produces a
// clear audit trail, and nothing external needs to react to "a job was
// enqueued" the way other domains' create events matter.
type Repository interface {
	Create(ctx context.Context, createdBy string, in EnqueueInput, runAt time.Time, maxAttempts int, recurrence *time.Duration) (Job, error)
	Get(ctx context.Context, id string) (Job, error)
	ListForCaller(ctx context.Context, callerID string, params ListParams) (ListResult, error)
	ListAttempts(ctx context.Context, jobID string) ([]JobAttempt, error)
}
