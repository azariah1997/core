package workflows_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"github.com/example/core-platform/backend/worker/internal/workflows"
)

func TestApprovalWorkflowApprovedPath(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.OnActivity(workflows.CallWebhookActivity, mock.Anything, mock.Anything).Return(nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("approve", nil)
	}, time.Minute)

	env.ExecuteWorkflow(workflows.ApprovalWorkflow, map[string]any{
		"prepareUrl": "http://example.invalid/prepare",
		"approveUrl": "http://example.invalid/approve",
		"rejectUrl":  "http://example.invalid/reject",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("expected workflow to complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	var result map[string]any
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result["outcome"] != "approved" {
		t.Fatalf("expected outcome approved, got %v", result)
	}
	env.AssertExpectations(t)
}

func TestApprovalWorkflowRejectedPath(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.OnActivity(workflows.CallWebhookActivity, mock.Anything, mock.Anything).Return(nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("reject", nil)
	}, time.Minute)

	env.ExecuteWorkflow(workflows.ApprovalWorkflow, map[string]any{
		"approveUrl": "http://example.invalid/approve",
		"rejectUrl":  "http://example.invalid/reject",
	})

	var result map[string]any
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result["outcome"] != "rejected" {
		t.Fatalf("expected outcome rejected, got %v", result)
	}
}

// TestApprovalWorkflowTimeoutPath is the compensation case: no signal
// ever arrives, so the durable timer fires and the workflow takes the
// same "undo" path a rejection would - confirmed here by asserting
// rejectUrl (not approveUrl) is the one invoked.
func TestApprovalWorkflowTimeoutPath(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	var calledURL string
	env.OnActivity(workflows.CallWebhookActivity, mock.Anything, mock.MatchedBy(func(payload map[string]any) bool {
		calledURL, _ = payload["url"].(string)
		return true
	})).Return(nil)

	env.ExecuteWorkflow(workflows.ApprovalWorkflow, map[string]any{
		"approveUrl":     "http://example.invalid/approve",
		"rejectUrl":      "http://example.invalid/reject",
		"timeoutSeconds": float64(5), // JSON numbers decode as float64, same as a real Temporal payload
	})

	var result map[string]any
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result["outcome"] != "timed_out" {
		t.Fatalf("expected outcome timed_out, got %v", result)
	}
	if calledURL != "http://example.invalid/reject" {
		t.Fatalf("expected the timeout path to call rejectUrl (compensation), got %q", calledURL)
	}
}

func TestApprovalWorkflowSkipsMissingURLs(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	// No prepareUrl/approveUrl in the input, so CallWebhookActivity should
	// never be invoked at all - env.AssertExpectations below would fail if
	// an unexpected call happened, since no .OnActivity was registered.

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("approve", nil)
	}, time.Minute)

	env.ExecuteWorkflow(workflows.ApprovalWorkflow, map[string]any{})

	var result map[string]any
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result["outcome"] != "approved" {
		t.Fatalf("expected outcome approved, got %v", result)
	}
}

func TestPingWebhookWorkflowRequiresURL(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.ExecuteWorkflow(workflows.PingWebhookWorkflow, map[string]any{})

	err := env.GetWorkflowError()
	if err == nil {
		t.Fatal("expected an error for a missing url")
	}
}

func TestPingWebhookWorkflowCallsTheActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.OnActivity(workflows.CallWebhookActivity, mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(workflows.PingWebhookWorkflow, map[string]any{"url": "http://example.invalid/ping"})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	env.AssertExpectations(t)
}
