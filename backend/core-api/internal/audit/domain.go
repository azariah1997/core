// Package audit is the platform's central Audit Service. "Audit must be
// immutable from normal application APIs" is the roadmap's own explicit
// requirement, enforced two ways: this package has no Update or Delete
// method or route at all (the strongest form of "can't misuse an API
// that doesn't exist"), and the underlying table has a database-level
// trigger rejecting UPDATE/DELETE outright (see
// data/migrations/0017_audit.sql) - defense in depth beyond just "the Go
// code never calls it."
package audit

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Record is one immutable audit entry, covering every field the roadmap
// names: actor, action, resource, timestamp, correlation ID,
// application, tenant, device context, and metadata. TenantID and
// DeviceID are the two the roadmap itself calls conditional ("when
// appropriate"), hence optional.
type Record struct {
	ID            string
	ActorUserID   string // empty means system-initiated, not a caller-attributable action
	Action        string // product/platform-defined, e.g. "role.assigned", free-form like every other Type/Action field in this repo
	ResourceType  string
	ResourceID    string
	AppID         string
	TenantID      string
	DeviceID      string
	CorrelationID string // always taken from context, never caller-supplied - see Service.Record
	Metadata      map[string]any
	OccurredAt    time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("audit record not found")
	ErrForbidden = errors.New("not permitted to access audit records")
)

// RecordInput deliberately has no OccurredAt or CorrelationID field -
// both are set server-side (OccurredAt to now(), CorrelationID from
// context), never caller-supplied, so an audit trail can't be backdated
// or have its correlation spoofed.
type RecordInput struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	AppID        string
	TenantID     string
	DeviceID     string
	Metadata     map[string]any
}

func (in RecordInput) Validate() error {
	switch {
	case strings.TrimSpace(in.Action) == "":
		return &ValidationError{"action is required"}
	case strings.TrimSpace(in.ResourceType) == "":
		return &ValidationError{"resourceType is required"}
	case strings.TrimSpace(in.ResourceID) == "":
		return &ValidationError{"resourceId is required"}
	}
	return nil
}

type ListFilter struct {
	AppID        string
	ActorUserID  string
	ResourceType string
	ResourceID   string
	Action       string
	Limit        int
	Cursor       string
}

type ListResult struct {
	Items      []Record
	NextCursor string
}

// Repository is the storage-agnostic boundary. Note what's absent:
// Update and Delete methods don't exist - immutability enforced by the
// interface itself having no way to violate it, not just by convention.
type Repository interface {
	Record(ctx context.Context, in RecordInput, correlationID string) (Record, error)
	Get(ctx context.Context, id string) (Record, error)
	List(ctx context.Context, filter ListFilter) (ListResult, error)
}
