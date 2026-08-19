package privacy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// WorkflowStarter matches internal/workflows/temporal.Client's Start
// method exactly, so that adapter is reused directly here with zero
// glue code - the same "satisfied directly, no adapter needed" pattern
// as AdminChecker, just for a different concrete type.
type WorkflowStarter interface {
	Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (runID string, err error)
}

// ExportStore is the narrow surface Service needs to persist an export
// bundle - satisfied directly by *files/s3.Store, which already has
// PresignGet and gains PutObject in this phase (files itself never
// needed a server-side PUT before now: every write it does is a
// presigned URL the client uploads to directly).
type ExportStore interface {
	PutObject(ctx context.Context, objectKey string, body []byte, contentType string) error
	PresignGet(ctx context.Context, objectKey string, expires time.Duration) (url string, err error)
}

const (
	defaultDownloadTTL = 15 * time.Minute
	maxRetentionList   = 500
)

type Service struct {
	repo      Repository
	admin     AdminChecker
	starter   WorkflowStarter
	store     ExportStore
	exporters map[string]Exporter
	deleters  map[string]Deleter
}

func NewService(repo Repository, admin AdminChecker, starter WorkflowStarter, store ExportStore) *Service {
	return &Service{
		repo: repo, admin: admin, starter: starter, store: store,
		exporters: map[string]Exporter{},
		deleters:  map[string]Deleter{},
	}
}

// RegisterExporter/RegisterDeleter are how other modules opt into
// ExportUserData/DeleteUserData - called from cmd/server/main.go once
// per participating module, after every domain service already exists.
// This registry is the actual point of this phase's cross-module
// requirement: adding a new participant later (e.g. notifications, a
// deliberately deferred one - see README.md) means writing one small
// adapter and two lines in main.go, not touching this package at all.
func (s *Service) RegisterExporter(name string, e Exporter) { s.exporters[name] = e }
func (s *Service) RegisterDeleter(name string, d Deleter)   { s.deleters[name] = d }

// SetConsent always inserts a new row - see domain.go's comment on why
// consent is append-only history, never an update.
func (s *Service) SetConsent(ctx context.Context, callerID string, in ConsentInput) (Consent, error) {
	if err := in.Validate(); err != nil {
		return Consent{}, err
	}
	c := Consent{ID: uuid.NewString(), UserID: callerID, Purpose: in.Purpose, Granted: in.Granted, Version: in.Version, RecordedAt: time.Now().UTC()}
	if c.Version == "" {
		c.Version = "1"
	}
	if err := s.repo.RecordConsent(ctx, c); err != nil {
		return Consent{}, err
	}
	return c, nil
}

// ListConsent is self-only - a user's consent history is exactly as
// sensitive as the rest of their profile, no admin-browsing use case was
// named for this phase (unlike audit's platform.admin-only trail, which
// exists precisely so admins CAN browse).
func (s *Service) ListConsent(ctx context.Context, callerID string) ([]Consent, error) {
	return s.repo.ListConsent(ctx, callerID)
}

func (s *Service) GetPreferences(ctx context.Context, callerID string) (map[string]bool, error) {
	return s.repo.GetPreferences(ctx, callerID)
}

func (s *Service) SetPreferences(ctx context.Context, callerID string, prefs map[string]bool) (map[string]bool, error) {
	if err := s.repo.SetPreferences(ctx, callerID, prefs); err != nil {
		return nil, err
	}
	return s.repo.GetPreferences(ctx, callerID)
}

func (s *Service) CreateRetentionPolicy(ctx context.Context, callerID string, in RetentionPolicyInput) (RetentionPolicy, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return RetentionPolicy{}, err
	}
	if err := in.Validate(); err != nil {
		return RetentionPolicy{}, err
	}
	now := time.Now().UTC()
	p := RetentionPolicy{
		ID: uuid.NewString(), AppID: in.AppID, ResourceType: in.ResourceType, RetentionDays: in.RetentionDays,
		CreatedBy: callerID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.UpsertRetentionPolicy(ctx, p); err != nil {
		return RetentionPolicy{}, err
	}
	return p, nil
}

func (s *Service) ListRetentionPolicies(ctx context.Context, callerID string) ([]RetentionPolicy, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListRetentionPolicies(ctx)
}

// RequestExport creates the tracking row first, then starts the durable
// workflow - if starting fails (Temporal unreachable), the row is marked
// failed immediately rather than left pending forever with nothing ever
// going to retry it.
func (s *Service) RequestExport(ctx context.Context, callerID string) (ExportRequest, error) {
	req := ExportRequest{ID: uuid.NewString(), UserID: callerID, Status: StatusPending, RequestedAt: time.Now().UTC()}
	if err := s.repo.CreateExportRequest(ctx, req); err != nil {
		return ExportRequest{}, err
	}
	workflowID := "privacy-export-" + req.ID
	runID, err := s.starter.Start(ctx, workflowID, ExportWorkflowType, TaskQueue, "", map[string]any{"requestId": req.ID, "userId": callerID})
	if err != nil {
		_ = s.repo.CompleteExportRequest(ctx, req.ID, StatusFailed, "", err.Error())
		return ExportRequest{}, err
	}
	if err := s.repo.SetExportWorkflowRef(ctx, req.ID, workflowID, runID); err != nil {
		return ExportRequest{}, err
	}
	// Re-fetch rather than patch the local struct: the embedded worker
	// (internal/privacy/workflow) can run and complete the activity
	// before this HTTP handler even returns, since it's in the same
	// process - a hand-patched "running" would understate real progress
	// exactly as often as it overstates it.
	return s.repo.GetExportRequest(ctx, req.ID)
}

func (s *Service) GetExportRequest(ctx context.Context, callerID, id string) (ExportRequest, error) {
	req, err := s.repo.GetExportRequest(ctx, id)
	if err != nil {
		return ExportRequest{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, req.UserID); err != nil {
		return ExportRequest{}, err
	}
	return req, nil
}

// GetExportDownloadURL only ever hands out a presigned GET, never the
// bundle bytes directly through this service - the same "the client
// talks to storage, not through us" principle as Phase 13's files
// module.
func (s *Service) GetExportDownloadURL(ctx context.Context, callerID, id string) (string, error) {
	req, err := s.GetExportRequest(ctx, callerID, id)
	if err != nil {
		return "", err
	}
	if req.Status != StatusCompleted {
		return "", ErrNotReady
	}
	return s.store.PresignGet(ctx, req.ObjectKey, defaultDownloadTTL)
}

func (s *Service) RequestDeletion(ctx context.Context, callerID string) (DeletionRequest, error) {
	req := DeletionRequest{ID: uuid.NewString(), UserID: callerID, Status: StatusPending, RequestedAt: time.Now().UTC()}
	if err := s.repo.CreateDeletionRequest(ctx, req); err != nil {
		return DeletionRequest{}, err
	}
	workflowID := "privacy-delete-" + req.ID
	runID, err := s.starter.Start(ctx, workflowID, DeleteWorkflowType, TaskQueue, "", map[string]any{"requestId": req.ID, "userId": callerID})
	if err != nil {
		_ = s.repo.CompleteDeletionRequest(ctx, req.ID, StatusFailed, map[string]any{"error": err.Error()})
		return DeletionRequest{}, err
	}
	if err := s.repo.SetDeletionWorkflowRef(ctx, req.ID, workflowID, runID); err != nil {
		return DeletionRequest{}, err
	}
	// Same re-fetch-don't-patch reasoning as RequestExport, above.
	return s.repo.GetDeletionRequest(ctx, req.ID)
}

func (s *Service) GetDeletionRequest(ctx context.Context, callerID, id string) (DeletionRequest, error) {
	req, err := s.repo.GetDeletionRequest(ctx, id)
	if err != nil {
		return DeletionRequest{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, req.UserID); err != nil {
		return DeletionRequest{}, err
	}
	return req, nil
}

// RunExport is called by the Temporal activity (internal/privacy/
// workflow), never directly by HTTP - it fans out to every registered
// Exporter, tolerating individual failures (one module's export error
// lands in the bundle as an "error" entry for that module rather than
// failing the whole export - a user's data from every OTHER module
// should still be retrievable even if one participant is down).
func (s *Service) RunExport(ctx context.Context, requestID, userID string) (map[string]any, error) {
	bundle := map[string]any{}
	for name, exp := range s.exporters {
		data, err := exp.ExportUserData(ctx, userID)
		if err != nil {
			bundle[name] = map[string]any{"error": err.Error()}
			continue
		}
		bundle[name] = data
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		_ = s.repo.CompleteExportRequest(ctx, requestID, StatusFailed, "", err.Error())
		return nil, err
	}
	// A deterministic key per request, not per attempt - a Temporal retry
	// of this same activity overwrites the same object rather than
	// littering storage with partial duplicates.
	objectKey := "privacy-exports/" + userID + "/" + requestID + ".json"
	if err := s.store.PutObject(ctx, objectKey, body, "application/json"); err != nil {
		_ = s.repo.CompleteExportRequest(ctx, requestID, StatusFailed, "", err.Error())
		return nil, err
	}
	if err := s.repo.CompleteExportRequest(ctx, requestID, StatusCompleted, objectKey, ""); err != nil {
		return nil, err
	}
	return map[string]any{"objectKey": objectKey, "participants": len(s.exporters)}, nil
}

// RunDeletion is called by the Temporal activity, same as RunExport.
// Every registered Deleter is attempted regardless of earlier failures -
// coordinated, not all-or-nothing: a user who is, say, also blocked by a
// storage outage on Files should still have their Devices and Users data
// erased rather than none of it, with the partial outcome recorded in
// Results for an admin to see and retry.
func (s *Service) RunDeletion(ctx context.Context, requestID, userID string) (map[string]any, error) {
	results := map[string]any{}
	anyFailed := false
	for name, del := range s.deleters {
		if err := del.DeleteUserData(ctx, userID); err != nil {
			results[name] = map[string]any{"status": "failed", "error": err.Error()}
			anyFailed = true
			continue
		}
		results[name] = map[string]any{"status": "deleted"}
	}
	status := StatusCompleted
	if anyFailed {
		status = StatusFailed
	}
	if err := s.repo.CompleteDeletionRequest(ctx, requestID, status, results); err != nil {
		return nil, err
	}
	return results, nil
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

func (s *Service) requireAdmin(ctx context.Context, callerID string) error {
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
