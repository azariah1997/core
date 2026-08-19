// Package privacy implements Phase 20: consent, user data export, user
// deletion, retention policy, data lifecycle, and privacy preferences -
// "each module must be capable of participating in ExportUserData/
// DeleteUserData" is the roadmap's own requirement, satisfied here via a
// small registry (Exporter/Deleter, below) that other modules' Service
// types are wired into from cmd/server/main.go, not by this package
// reaching into their internals.
//
// "Do not create one giant cross-module delete query. Coordinate
// deletion through workflows/events" is the roadmap's other explicit
// instruction - see internal/privacy/workflow for how that's satisfied.
package privacy

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Temporal coupling points for Phase 20's export/deletion workflows -
// defined here (the "caller" side, cmd/server/main.go's Service.Start)
// rather than in internal/privacy/workflow (the "definer" side) so that
// package can import this one for them without a cycle; see its own doc
// comment for the full reasoning.
const (
	TaskQueue          = "privacy-tasks"
	ExportWorkflowType = "privacy.export_user_data"
	DeleteWorkflowType = "privacy.delete_user_data"
)

type RequestStatus string

const (
	StatusPending   RequestStatus = "pending"
	StatusRunning   RequestStatus = "running"
	StatusCompleted RequestStatus = "completed"
	StatusFailed    RequestStatus = "failed"
)

// Consent is one recorded grant/revoke decision for a purpose (e.g.
// "marketing_email", "analytics", "third_party_sharing" - purposes are
// product-defined free-form strings, the same "do not hardcode product
// concepts" convention as RelationshipType and Message.Type). Rows are
// never updated or deleted - see the migration's own comment - the
// current effective consent for a purpose is the newest row for it.
type Consent struct {
	ID         string
	UserID     string
	Purpose    string
	Granted    bool
	Version    string
	RecordedAt time.Time
}

type ConsentInput struct {
	Purpose string
	Granted bool
	Version string
}

func (in ConsentInput) Validate() error {
	if strings.TrimSpace(in.Purpose) == "" {
		return &ValidationError{"purpose is required"}
	}
	return nil
}

// RetentionPolicy is one operational rule: rows of ResourceType (also a
// free-form, product-defined string, e.g. "message", "notification")
// belonging to AppID (nil means platform-wide) should not be kept longer
// than RetentionDays. This package does not enforce retention itself -
// same documented gap as Phase 13's PurgeExpired, callable but not
// scheduled - it's a declared policy a module or a future scheduler
// reads and acts on, not an automatic purge engine.
type RetentionPolicy struct {
	ID            string
	AppID         string
	ResourceType  string
	RetentionDays int
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RetentionPolicyInput struct {
	AppID         string
	ResourceType  string
	RetentionDays int
}

func (in RetentionPolicyInput) Validate() error {
	if strings.TrimSpace(in.ResourceType) == "" {
		return &ValidationError{"resourceType is required"}
	}
	if in.RetentionDays <= 0 {
		return &ValidationError{"retentionDays must be positive"}
	}
	return nil
}

// ExportRequest and DeletionRequest are Postgres-status-is-truth records
// - unlike Phase 16's WorkflowRun (which deliberately never duplicates
// Temporal's execution state), these two DO persist their own status,
// because the caller-facing contract here is "is my request done," not
// "let me inspect a workflow" - Temporal is purely the internal engine
// coordinating each participant call, invisible to every HTTP response
// in this package. See workflow/workflow.go's doc comment for why
// Temporal is used at all despite that.
type ExportRequest struct {
	ID          string
	UserID      string
	Status      RequestStatus
	WorkflowID  string
	RunID       string
	ObjectKey   string
	Error       string
	RequestedAt time.Time
	CompletedAt *time.Time
}

type DeletionRequest struct {
	ID          string
	UserID      string
	Status      RequestStatus
	WorkflowID  string
	RunID       string
	Results     map[string]any
	Error       string
	RequestedAt time.Time
	CompletedAt *time.Time
}

// Exporter and Deleter are the two halves of "each module must be
// capable of participating in ExportUserData/DeleteUserData," kept
// separate rather than one combined interface specifically so a
// participant like audit (immutable by design, Phase 19) can implement
// Exporter alone - there is no fake no-op DeleteUserData method to
// write or forget to call; the type system itself expresses "this
// module's data can be read back but never erased."
type Exporter interface {
	ExportUserData(ctx context.Context, userID string) (map[string]any, error)
}

type Deleter interface {
	DeleteUserData(ctx context.Context, userID string) error
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("privacy request not found")
	ErrForbidden = errors.New("not permitted to access this privacy request")
	ErrNotReady  = errors.New("export is not completed yet")
)

// Repository is the storage-agnostic boundary for everything in this
// package except the actual cross-module export/delete calls, which go
// through the Exporter/Deleter registry instead of storage.
type Repository interface {
	RecordConsent(ctx context.Context, c Consent) error
	ListConsent(ctx context.Context, userID string) ([]Consent, error)

	GetPreferences(ctx context.Context, userID string) (map[string]bool, error)
	SetPreferences(ctx context.Context, userID string, prefs map[string]bool) error

	UpsertRetentionPolicy(ctx context.Context, p RetentionPolicy) error
	ListRetentionPolicies(ctx context.Context) ([]RetentionPolicy, error)

	CreateExportRequest(ctx context.Context, r ExportRequest) error
	SetExportWorkflowRef(ctx context.Context, id, workflowID, runID string) error
	CompleteExportRequest(ctx context.Context, id string, status RequestStatus, objectKey, errMsg string) error
	GetExportRequest(ctx context.Context, id string) (ExportRequest, error)

	CreateDeletionRequest(ctx context.Context, r DeletionRequest) error
	SetDeletionWorkflowRef(ctx context.Context, id, workflowID, runID string) error
	CompleteDeletionRequest(ctx context.Context, id string, status RequestStatus, results map[string]any) error
	GetDeletionRequest(ctx context.Context, id string) (DeletionRequest, error)
}
