package workflows

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/packages/go/platformkit/workflowkit"
)

const taskQueue = workflowkit.TaskQueue

// TemporalClient is the narrow surface Service needs - satisfied by
// internal/workflows/temporal's real adapter over go.temporal.io/sdk's
// client.Client, and by a fake in tests. Kept this narrow (Start/
// Describe/Signal, not the full SDK client) so nothing outside the
// temporal adapter package ever imports go.temporal.io/sdk/client
// directly - the actual embodiment of "do not expose Temporal directly."
type TemporalClient interface {
	Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (runID string, err error)
	Describe(ctx context.Context, workflowID, runID string) (Execution, error)
	Signal(ctx context.Context, workflowID, runID, signalName string, payload map[string]any) error
}

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	repo     Repository
	temporal TemporalClient
	admin    AdminChecker
}

func NewService(repo Repository, temporal TemporalClient, admin AdminChecker) *Service {
	return &Service{repo: repo, temporal: temporal, admin: admin}
}

// Start begins a new workflow execution. The workflow ID is generated
// here (never caller-supplied) so ownership can never be spoofed by
// guessing or reusing someone else's workflow ID.
func (s *Service) Start(ctx context.Context, callerID string, in StartInput) (WorkflowRun, error) {
	if err := in.Validate(); err != nil {
		return WorkflowRun{}, err
	}
	workflowID := in.Type + "-" + uuid.NewString()
	runID, err := s.temporal.Start(ctx, workflowID, in.Type, taskQueue, in.CronSchedule, in.Payload)
	if err != nil {
		return WorkflowRun{}, err
	}
	run := WorkflowRun{WorkflowID: workflowID, RunID: runID, Type: in.Type, CreatedBy: callerID, CreatedAt: time.Now().UTC()}
	if err := s.repo.Create(ctx, run); err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func (s *Service) Get(ctx context.Context, callerID, workflowID string) (WorkflowRun, Execution, error) {
	run, err := s.repo.Get(ctx, workflowID)
	if err != nil {
		return WorkflowRun{}, Execution{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, run.CreatedBy); err != nil {
		return WorkflowRun{}, Execution{}, err
	}
	exec, err := s.temporal.Describe(ctx, run.WorkflowID, run.RunID)
	if err != nil {
		return WorkflowRun{}, Execution{}, err
	}
	return run, exec, nil
}

func (s *Service) Signal(ctx context.Context, callerID, workflowID string, in SignalInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	run, err := s.repo.Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, run.CreatedBy); err != nil {
		return err
	}
	return s.temporal.Signal(ctx, run.WorkflowID, run.RunID, in.Name, in.Payload)
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
