// Package temporal implements workflows.TemporalClient against the real
// go.temporal.io/sdk client - the only place in core-api that imports the
// Temporal SDK directly. Nothing outside this package (and nothing
// outside core-api entirely) ever touches Temporal itself.
package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"

	"github.com/example/core-platform/backend/core-api/internal/workflows"
)

type Config struct {
	HostPort string
}

type Client struct {
	client client.Client
}

func New(cfg Config) (*Client, error) {
	c, err := client.Dial(client.Options{HostPort: cfg.HostPort})
	if err != nil {
		return nil, fmt.Errorf("dial temporal: %w", err)
	}
	return &Client{client: c}, nil
}

func (c *Client) Close() {
	c.client.Close()
}

func (c *Client) Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (string, error) {
	opts := client.StartWorkflowOptions{ID: workflowID, TaskQueue: taskQueue}
	if cronSchedule != "" {
		opts.CronSchedule = cronSchedule
	}
	run, err := c.client.ExecuteWorkflow(ctx, opts, workflowType, payload)
	if err != nil {
		return "", fmt.Errorf("start workflow: %w", err)
	}
	return run.GetRunID(), nil
}

// stringer is satisfied by Temporal's WorkflowExecutionStatus enum type
// without this package needing to import go.temporal.io/api directly
// just for one type reference.
type stringer interface{ String() string }

func mapStatus(s stringer) workflows.Status {
	switch s.String() {
	case "Running", "ContinuedAsNew":
		return workflows.StatusRunning
	case "Completed":
		return workflows.StatusCompleted
	case "Failed":
		return workflows.StatusFailed
	case "Canceled":
		return workflows.StatusCanceled
	case "Terminated":
		return workflows.StatusTerminated
	case "TimedOut":
		return workflows.StatusTimedOut
	default:
		return workflows.StatusUnknown
	}
}

// Describe reports the workflow's live status from Temporal - never a
// cached copy. For a terminal, successful completion, it also fetches
// the result; GetWorkflow(...).Get() on an already-finished execution
// returns immediately from Temporal's cached completion event, it does
// not re-block waiting for anything.
func (c *Client) Describe(ctx context.Context, workflowID, runID string) (workflows.Execution, error) {
	resp, err := c.client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return workflows.Execution{}, fmt.Errorf("describe workflow: %w", err)
	}
	status := mapStatus(resp.GetWorkflowExecutionInfo().GetStatus())
	exec := workflows.Execution{Status: status}

	switch status {
	case workflows.StatusCompleted:
		var result map[string]any
		if getErr := c.client.GetWorkflow(ctx, workflowID, runID).Get(ctx, &result); getErr != nil {
			exec.Status = workflows.StatusFailed
			exec.Error = getErr.Error()
			return exec, nil
		}
		exec.Result = result
	case workflows.StatusFailed, workflows.StatusTimedOut:
		if getErr := c.client.GetWorkflow(ctx, workflowID, runID).Get(ctx, nil); getErr != nil {
			exec.Error = getErr.Error()
		}
	}
	return exec, nil
}

func (c *Client) Signal(ctx context.Context, workflowID, runID, signalName string, payload map[string]any) error {
	if err := c.client.SignalWorkflow(ctx, workflowID, runID, signalName, payload); err != nil {
		return fmt.Errorf("signal workflow: %w", err)
	}
	return nil
}
