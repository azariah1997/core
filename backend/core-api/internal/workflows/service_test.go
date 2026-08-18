package workflows_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/workflows"
	"github.com/example/core-platform/backend/core-api/internal/workflows/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeTemporal struct {
	started   map[string]string // workflowID -> workflowType
	taskQueue string
	cron      map[string]string
	signals   []signalCall
	execution workflows.Execution
}

type signalCall struct {
	workflowID, signalName string
	payload                map[string]any
}

func newFakeTemporal() *fakeTemporal {
	return &fakeTemporal{started: map[string]string{}, cron: map[string]string{}, execution: workflows.Execution{Status: workflows.StatusRunning}}
}

func (f *fakeTemporal) Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (string, error) {
	f.started[workflowID] = workflowType
	f.taskQueue = taskQueue
	if cronSchedule != "" {
		f.cron[workflowID] = cronSchedule
	}
	return "run-" + workflowID, nil
}

func (f *fakeTemporal) Describe(ctx context.Context, workflowID, runID string) (workflows.Execution, error) {
	return f.execution, nil
}

func (f *fakeTemporal) Signal(ctx context.Context, workflowID, runID, signalName string, payload map[string]any) error {
	f.signals = append(f.signals, signalCall{workflowID: workflowID, signalName: signalName, payload: payload})
	return nil
}

func newService(temporal *fakeTemporal, admins map[string]bool) *workflows.Service {
	return workflows.NewService(memory.New(), temporal, fakeAdmin{admins: admins})
}

func TestStartRequiresType(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	_, err := svc.Start(context.Background(), "u1", workflows.StartInput{})
	var verr *workflows.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestStartRoutesToTheSharedTaskQueue(t *testing.T) {
	temporal := newFakeTemporal()
	svc := newService(temporal, nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if temporal.started[run.WorkflowID] != "approval" {
		t.Fatalf("expected the workflow to be started with type 'approval', got %v", temporal.started)
	}
	if temporal.taskQueue == "" {
		t.Fatal("expected a non-empty task queue")
	}
}

func TestStartWithCronSchedulePassesItThrough(t *testing.T) {
	temporal := newFakeTemporal()
	svc := newService(temporal, nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "ping_webhook", CronSchedule: "*/5 * * * *"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if temporal.cron[run.WorkflowID] != "*/5 * * * *" {
		t.Fatalf("expected cron schedule to be passed through, got %v", temporal.cron)
	}
}

func TestNonOwnerCannotGetWorkflow(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := svc.Get(context.Background(), "stranger", run.WorkflowID); !errors.Is(err, workflows.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAdminCanGetAnyonesWorkflow(t *testing.T) {
	svc := newService(newFakeTemporal(), map[string]bool{"admin": true})
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := svc.Get(context.Background(), "admin", run.WorkflowID); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

func TestGetReturnsLiveExecutionStateFromTemporal(t *testing.T) {
	temporal := newFakeTemporal()
	temporal.execution = workflows.Execution{Status: workflows.StatusCompleted, Result: map[string]any{"outcome": "approved"}}
	svc := newService(temporal, nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_, exec, err := svc.Get(context.Background(), "u1", run.WorkflowID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if exec.Status != workflows.StatusCompleted || exec.Result["outcome"] != "approved" {
		t.Fatalf("unexpected execution: %+v", exec)
	}
}

func TestSignalRequiresName(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	err = svc.Signal(context.Background(), "u1", run.WorkflowID, workflows.SignalInput{})
	var verr *workflows.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestNonOwnerCannotSignal(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	err = svc.Signal(context.Background(), "stranger", run.WorkflowID, workflows.SignalInput{Name: "approve"})
	if !errors.Is(err, workflows.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestOwnerCanSignal(t *testing.T) {
	temporal := newFakeTemporal()
	svc := newService(temporal, nil)
	run, err := svc.Start(context.Background(), "u1", workflows.StartInput{Type: "approval"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Signal(context.Background(), "u1", run.WorkflowID, workflows.SignalInput{Name: "approve"}); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if len(temporal.signals) != 1 || temporal.signals[0].signalName != "approve" {
		t.Fatalf("expected a recorded approve signal, got %+v", temporal.signals)
	}
}
