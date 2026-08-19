// Package workflow holds the Temporal workflow/activity definitions for
// Phase 20's coordinated user-data export and deletion, plus the small
// embedded Temporal worker that executes them - INSIDE core-api itself,
// unlike every other workflow in this repo (Phase 16's ApprovalWorkflow/
// PingWebhookWorkflow, registered by the separate `worker` service on
// the shared workflowkit.TaskQueue).
//
// That is a deliberate, different architectural choice, not an
// inconsistency. worker is a separate Go module and cannot import
// core-api's internal domain packages - the same constraint Phase 14's
// search indexer documented as the reason its indexed documents stay
// thin. Exporting or deleting a user's data means calling directly into
// core-api's own users/devices/files/audit services (see internal/api's
// privacy adapters and cmd/server/main.go's registration calls) - only a
// worker living inside core-api's own process/module can do that.
// Temporal still earns its keep here for the same reason Phase 16 chose
// it: durable, retryable, multi-step execution that survives a process
// restart mid-run - "coordinate deletion through workflows/events," in
// the roadmap's own words. It is simply core-api running its own worker
// on its own task queue (privacy.TaskQueue) instead of delegating to the
// shared one, the same way a large system typically has more than one
// worker pool, each owned by whichever service can actually do the work.
//
// This is also the second (and only other) place in core-api that
// imports the Temporal SDK directly, alongside internal/workflows/
// temporal - that package only ever needs a client (Start/Describe/
// Signal); this one also needs to run a worker.Worker, since it has to
// actually execute code, not just ask Temporal to.
package workflow

import (
	"context"
	"time"

	temporalclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	temporalworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/example/core-platform/backend/core-api/internal/privacy"
)

const activityTimeout = 5 * time.Minute

// Runner is the narrow surface these activities need from
// privacy.Service - satisfied directly by *privacy.Service (RunExport/
// RunDeletion), no adapter needed, the same consumer-defined-interface
// pattern as every other cross-package boundary in this repo.
type Runner interface {
	RunExport(ctx context.Context, requestID, userID string) (map[string]any, error)
	RunDeletion(ctx context.Context, requestID, userID string) (map[string]any, error)
}

type handlers struct{ runner Runner }

func (h *handlers) ExportActivity(ctx context.Context, input map[string]any) (map[string]any, error) {
	requestID, _ := input["requestId"].(string)
	userID, _ := input["userId"].(string)
	return h.runner.RunExport(ctx, requestID, userID)
}

func (h *handlers) DeleteActivity(ctx context.Context, input map[string]any) (map[string]any, error) {
	requestID, _ := input["requestId"].(string)
	userID, _ := input["userId"].(string)
	return h.runner.RunDeletion(ctx, requestID, userID)
}

func withRetry(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityTimeout,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})
}

// ExportUserDataWorkflow and DeleteUserDataWorkflow are each a single
// retryable activity call - deliberately trivial as workflows go (the
// same triviality Phase 16's PingWebhookWorkflow has), because the real
// coordination logic (fan out to every registered participant, tolerate
// individual failures, record the outcome) lives in privacy.Service.Run
// Export/RunDeletion, testable on its own with plain fakes and no
// Temporal test harness required. What Temporal adds here is exactly
// what a bare Go function call couldn't: automatic retry with backoff if
// a participant call fails transiently, and durability if core-api
// itself restarts mid-run.
func ExportUserDataWorkflow(ctx workflow.Context, input map[string]any) (map[string]any, error) {
	ctx = withRetry(ctx)
	var h *handlers
	var result map[string]any
	err := workflow.ExecuteActivity(ctx, h.ExportActivity, input).Get(ctx, &result)
	return result, err
}

func DeleteUserDataWorkflow(ctx workflow.Context, input map[string]any) (map[string]any, error) {
	ctx = withRetry(ctx)
	var h *handlers
	var result map[string]any
	err := workflow.ExecuteActivity(ctx, h.DeleteActivity, input).Get(ctx, &result)
	return result, err
}

func register(w temporalworker.Worker, runner Runner) {
	h := &handlers{runner: runner}
	w.RegisterWorkflowWithOptions(ExportUserDataWorkflow, workflow.RegisterOptions{Name: privacy.ExportWorkflowType})
	w.RegisterWorkflowWithOptions(DeleteUserDataWorkflow, workflow.RegisterOptions{Name: privacy.DeleteWorkflowType})
	w.RegisterActivity(h.ExportActivity)
	w.RegisterActivity(h.DeleteActivity)
}

// StartWorker dials Temporal and starts a dedicated worker on
// privacy.TaskQueue, confined to this package the same way
// internal/workflows/temporal confines the SDK on the client side.
// Returns a stop function cmd/server/main.go defers alongside every
// other resource it owns.
func StartWorker(hostPort string, runner Runner) (stop func(), err error) {
	conn, err := temporalclient.Dial(temporalclient.Options{HostPort: hostPort})
	if err != nil {
		return nil, err
	}
	w := temporalworker.New(conn, privacy.TaskQueue, temporalworker.Options{})
	register(w, runner)
	if err := w.Start(); err != nil {
		conn.Close()
		return nil, err
	}
	return func() {
		w.Stop()
		conn.Close()
	}, nil
}
