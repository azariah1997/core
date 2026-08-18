// Package workflows is the Core Workflow API/SDK the roadmap asks for -
// "do not expose Temporal directly to product applications" is its whole
// reason to exist. Callers Start/Get/Signal by workflow type name and
// workflow ID; Temporal itself, and the actual workflow/activity code
// (backend/worker/internal/workflows), are never reachable from outside
// this package.
package workflows

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Status is this package's own vocabulary, translated from whatever
// Temporal's DescribeWorkflowExecution reports - callers depend on this
// package's contract, not Temporal's wire types, the same layering as
// every other module wrapping an external system in this repo.
type Status string

const (
	StatusRunning    Status = "running"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCanceled   Status = "canceled"
	StatusTerminated Status = "terminated"
	StatusTimedOut   Status = "timed_out"
	StatusUnknown    Status = "unknown"
)

// WorkflowRun is this package's own bookkeeping record - who started
// which workflow type, for authorization (self-or-admin, same pattern as
// every other module). Execution state (status, result, history) is
// never duplicated here; it's queried live from Temporal, which remains
// the source of truth for it, the same "don't duplicate the real source
// of truth" principle files applies to S3/MinIO and search applies to
// OpenSearch.
type WorkflowRun struct {
	WorkflowID string
	RunID      string
	Type       string
	CreatedBy  string
	CreatedAt  time.Time
}

// Execution is a live snapshot from Temporal, never persisted by this
// package.
type Execution struct {
	Status Status
	Result map[string]any // populated only when Status is a terminal, successful state
	Error  string         // populated only when Status is Failed
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("workflow not found")
	ErrForbidden = errors.New("not permitted to access this workflow")
)

type StartInput struct {
	Type         string
	Payload      map[string]any
	CronSchedule string // optional - Temporal's own native recurring-execution feature
}

func (in StartInput) Validate() error {
	if strings.TrimSpace(in.Type) == "" {
		return &ValidationError{"type is required"}
	}
	return nil
}

type SignalInput struct {
	Name    string
	Payload map[string]any
}

func (in SignalInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return &ValidationError{"name is required"}
	}
	return nil
}

// Repository is this package's own storage-agnostic boundary for
// WorkflowRun ownership records - never Temporal's execution state.
type Repository interface {
	Create(ctx context.Context, run WorkflowRun) error
	Get(ctx context.Context, workflowID string) (WorkflowRun, error)
}
