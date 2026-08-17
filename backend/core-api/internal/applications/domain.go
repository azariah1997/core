// Package applications implements the Application Registry: the platform's
// generic, product-neutral record of which applications exist. It never
// assumes any particular product type.
package applications

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusArchived Status = "archived"
)

func (s Status) valid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusArchived:
		return true
	default:
		return false
	}
}

type Application struct {
	ID        string
	Slug      string
	Name      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ValidationError signals a client input problem, distinct from
// infrastructure failures, so callers can map it to a 4xx response.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("application not found")
	ErrSlugTaken = errors.New("slug already in use")
)

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	idPattern   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

func ValidID(id string) bool { return idPattern.MatchString(id) }

type CreateInput struct {
	Slug string
	Name string
}

func (in CreateInput) Validate() error {
	slug := strings.TrimSpace(in.Slug)
	name := strings.TrimSpace(in.Name)
	switch {
	case slug == "":
		return &ValidationError{"slug is required"}
	case !slugPattern.MatchString(slug):
		return &ValidationError{"slug must be lowercase alphanumeric with optional internal hyphens"}
	case name == "":
		return &ValidationError{"name is required"}
	}
	return nil
}

type UpdateInput struct {
	Name   *string
	Status *Status
}

func (in UpdateInput) Validate() error {
	if in.Name == nil && in.Status == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return &ValidationError{"name cannot be empty"}
	}
	if in.Status != nil && !in.Status.valid() {
		return &ValidationError{"status must be one of active, inactive, archived"}
	}
	return nil
}

// ListParams are cursor-based: Cursor is an opaque token from a previous
// ListResult.NextCursor, never an offset, so pages stay stable under
// concurrent inserts.
type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []Application
	NextCursor string
}

// Repository is the storage-agnostic boundary the service depends on.
// Implementations (postgres, memory) own event emission for writes so it
// happens atomically with the underlying persistence change.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (Application, error)
	Get(ctx context.Context, id string) (Application, error)
	List(ctx context.Context, params ListParams) (ListResult, error)
	Update(ctx context.Context, id string, in UpdateInput) (Application, error)
}
