// Package users implements User: the platform person/account, deliberately
// separate from Identity (who authenticated, Phase 3). No product-specific
// data belongs on User - only the generic profile fields every product
// needs (display name, avatar, locale, timezone) plus lifecycle status.
package users

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
	// StatusDeleted is a soft delete: the row is retained (for audit,
	// referential integrity, and eventual privacy-workflow processing in
	// Phase 20) but the user is gone from every normal API's perspective.
	StatusDeleted Status = "deleted"
)

func (s Status) settableViaUpdate() bool {
	switch s {
	case StatusActive, StatusDeactivated:
		return true
	default:
		return false
	}
}

type User struct {
	ID          string
	DisplayName string
	AvatarRef   string
	Locale      string
	Timezone    string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound = errors.New("user not found")
	// ErrDeleted signals the resource exists but is soft-deleted; treated
	// like ErrNotFound by callers, kept distinct for clearer logging.
	ErrDeleted = errors.New("user is deleted")
)

type CreateInput struct {
	DisplayName string
	Locale      string
	Timezone    string
}

func (in CreateInput) Validate() error {
	if strings.TrimSpace(in.DisplayName) == "" {
		return &ValidationError{"displayName is required"}
	}
	return nil
}

type UpdateInput struct {
	DisplayName *string
	AvatarRef   *string
	Locale      *string
	Timezone    *string
	// Status may only be set to active or deactivated here. Deletion is a
	// dedicated operation (Service.Delete) so it always emits user.deleted
	// rather than being one case among many in a generic update.
	Status *Status
}

func (in UpdateInput) Validate() error {
	if in.DisplayName == nil && in.AvatarRef == nil && in.Locale == nil && in.Timezone == nil && in.Status == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	if in.DisplayName != nil && strings.TrimSpace(*in.DisplayName) == "" {
		return &ValidationError{"displayName cannot be empty"}
	}
	if in.Status != nil && !in.Status.settableViaUpdate() {
		return &ValidationError{"status must be one of active, deactivated"}
	}
	return nil
}

// Repository is the storage-agnostic boundary. Update additionally emits
// user.deactivated instead of user.updated when Status transitions to
// deactivated, so lifecycle changes stay distinguishable in the event
// stream from ordinary profile edits.
type Repository interface {
	Create(ctx context.Context, in CreateInput) (User, error)
	Get(ctx context.Context, id string) (User, error)
	Update(ctx context.Context, id string, in UpdateInput) (User, error)
	Delete(ctx context.Context, id string) (User, error)
}
