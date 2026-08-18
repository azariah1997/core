// Package workflows holds the platform's built-in Temporal workflow and
// activity definitions - the "Core Workflow API/SDK" the roadmap asks
// for is core-api/internal/workflows; this package is what actually runs
// inside those durable executions, registered against worker's Temporal
// Worker (cmd/worker/main.go), never imported by core-api directly
// (core-api only ever talks to Temporal by workflow type *name*, exactly
// like it addresses background job types by name in Phase 15).
//
// Both workflows demonstrate every capability the roadmap names:
//   - multi-step process + retries: CallWebhookActivity runs under a
//     Temporal-native ActivityOptions.RetryPolicy - real automatic retry,
//     not the hand-rolled backoff Phase 15's job queue needed, because
//     Temporal provides it natively for anything running inside a
//     workflow.
//   - long-running operations: ApprovalWorkflow can wait indefinitely
//     for a signal via a durable timer that survives worker restarts.
//   - compensation: a reject signal or a timeout both run the same
//     "undo" activity a plain approval would have skipped.
//   - scheduled processes: PingWebhookWorkflow is meant to be started
//     with a Temporal CronSchedule (see core-api/internal/workflows),
//     Temporal's own native recurring-execution feature.
package workflows

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/example/core-platform/backend/worker/internal/jobrunner/handlers"
)

// CallWebhookActivity reuses Phase 15's real webhook-call logic (a
// genuine HTTP POST, not simulated) - the same honest capability serving
// two different durable-execution mechanisms (a job's own hand-rolled
// retry loop there, Temporal's native retry policy here) rather than two
// independent implementations of "POST this JSON somewhere."
func CallWebhookActivity(ctx context.Context, payload map[string]any) error {
	return handlers.Webhook(nil)(ctx, payload)
}

const activityTimeout = 30 * time.Second

func withRetryableActivity(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityTimeout,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})
}

// ApprovalWorkflow expects input with string keys "prepareUrl",
// "approveUrl", "rejectUrl" (each optional - a missing URL just skips
// that step) and a numeric "timeoutSeconds" (default 1 hour). It calls
// prepareUrl, then waits for an "approve" or "reject" signal (or the
// timeout, whichever comes first), then calls approveUrl on approval or
// rejectUrl otherwise - the reject path is the compensation for whatever
// prepareUrl did, run on both an explicit rejection and a timeout since
// both mean "this did not get approved in time."
func ApprovalWorkflow(ctx workflow.Context, input map[string]any) (map[string]any, error) {
	ctx = withRetryableActivity(ctx)

	if prepareURL, _ := input["prepareUrl"].(string); prepareURL != "" {
		if err := workflow.ExecuteActivity(ctx, CallWebhookActivity, map[string]any{"url": prepareURL, "body": input}).Get(ctx, nil); err != nil {
			return nil, fmt.Errorf("prepare step failed: %w", err)
		}
	}

	timeoutSeconds := 3600
	if v, ok := input["timeoutSeconds"].(float64); ok && v > 0 {
		timeoutSeconds = int(v)
	}

	approveCh := workflow.GetSignalChannel(ctx, "approve")
	rejectCh := workflow.GetSignalChannel(ctx, "reject")
	timer := workflow.NewTimer(ctx, time.Duration(timeoutSeconds)*time.Second)

	outcome := "timed_out"
	selector := workflow.NewSelector(ctx)
	selector.AddReceive(approveCh, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, nil)
		outcome = "approved"
	})
	selector.AddReceive(rejectCh, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, nil)
		outcome = "rejected"
	})
	selector.AddFuture(timer, func(f workflow.Future) {
		outcome = "timed_out"
	})
	selector.Select(ctx)

	finalURLKey := "rejectUrl"
	if outcome == "approved" {
		finalURLKey = "approveUrl"
	}
	var finalErr error
	if finalURL, _ := input[finalURLKey].(string); finalURL != "" {
		finalErr = workflow.ExecuteActivity(ctx, CallWebhookActivity, map[string]any{"url": finalURL, "body": map[string]any{"outcome": outcome}}).Get(ctx, nil)
	}
	return map[string]any{"outcome": outcome}, finalErr
}

// PingWebhookWorkflow calls input["url"] once. On its own it's a trivial
// single-activity workflow - its purpose is to be startable with a
// Temporal CronSchedule (see core-api/internal/workflows.Service.Start),
// demonstrating "scheduled processes" as a durable, server-side-native
// capability rather than a poll loop.
func PingWebhookWorkflow(ctx workflow.Context, input map[string]any) (map[string]any, error) {
	ctx = withRetryableActivity(ctx)
	url, _ := input["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("ping_webhook workflow requires a non-empty \"url\" in its input")
	}
	err := workflow.ExecuteActivity(ctx, CallWebhookActivity, map[string]any{"url": url, "body": input}).Get(ctx, nil)
	return map[string]any{"pinged": url}, err
}

// Workflow type names - what core-api's Service.Start (a free-form
// string, like every other Type field in this repo) must pass as
// StartInput.Type for Temporal to route to the corresponding function
// below. Registering under the Go function name instead of these
// explicit names would silently break that string-based routing: a
// client starting "approval" wouldn't match a worker that only knows
// "ApprovalWorkflow" (Temporal's default name when none is given), and
// the workflow would sit retrying "unable to find workflow type"
// forever, invisible to core-api's own Start caller since the start
// call itself succeeds - a real bug caught only in this phase's live
// validation, not any unit test.
const (
	ApprovalWorkflowType    = "approval"
	PingWebhookWorkflowType = "ping_webhook"
)

// Register wires every built-in workflow/activity onto w - called once
// from cmd/worker/main.go.
func Register(w worker.Worker) {
	w.RegisterWorkflowWithOptions(ApprovalWorkflow, workflow.RegisterOptions{Name: ApprovalWorkflowType})
	w.RegisterWorkflowWithOptions(PingWebhookWorkflow, workflow.RegisterOptions{Name: PingWebhookWorkflowType})
	w.RegisterActivity(CallWebhookActivity)
}
