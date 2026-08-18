// Package groups implements a generic grouping primitive: friend circles,
// families, teams, communities, workspaces, gaming guilds are all the same
// underlying shape to the platform. Nothing here hardcodes any one of
// those use cases - GroupMember.Role is a free-form, product-defined
// label (never validated against a fixed enum), and the only structural
// concept the platform itself enforces is IsManager: one permission bit,
// orthogonal to whatever the product calls that member's role.
package groups

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

func (s Status) valid() bool {
	switch s {
	case StatusActive, StatusArchived:
		return true
	default:
		return false
	}
}

type Group struct {
	ID        string
	AppID     string
	Name      string
	Status    Status
	Metadata  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GroupMember struct {
	ID        string
	GroupID   string
	UserID    string
	Role      string // product-defined; purely descriptive
	IsManager bool   // the one structural permission bit
	CreatedAt time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound           = errors.New("group not found")
	ErrMembershipNotFound = errors.New("membership not found")
	ErrAlreadyMember      = errors.New("user is already a member")
	ErrForbidden          = errors.New("not a member of this group")
	ErrManagerRequired    = errors.New("manager permission required")
)

type CreateInput struct {
	AppID    string
	Name     string
	Metadata map[string]any
}

func (in CreateInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Name) == "":
		return &ValidationError{"name is required"}
	}
	return nil
}

type UpdateInput struct {
	Name     *string
	Status   *Status
	Metadata map[string]any
}

func (in UpdateInput) Validate() error {
	if in.Name == nil && in.Status == nil && in.Metadata == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return &ValidationError{"name cannot be empty"}
	}
	if in.Status != nil && !in.Status.valid() {
		return &ValidationError{"status must be one of active, archived"}
	}
	return nil
}

type AddMemberInput struct {
	UserID    string
	Role      string
	IsManager bool
}

func (in AddMemberInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return &ValidationError{"userId is required"}
	}
	return nil
}

type UpdateMemberInput struct {
	Role      *string
	IsManager *bool
}

func (in UpdateMemberInput) Validate() error {
	if in.Role == nil && in.IsManager == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	return nil
}

// Repository is the storage-agnostic boundary.
type Repository interface {
	// Create creates the group and a manager Membership for creatorUserID
	// atomically - a group can never transiently exist with no manager.
	Create(ctx context.Context, creatorUserID string, in CreateInput) (Group, error)
	Get(ctx context.Context, id string) (Group, error)
	ListForUser(ctx context.Context, userID string) ([]Group, error)
	Update(ctx context.Context, id string, in UpdateInput) (Group, error)

	GetMembership(ctx context.Context, groupID, userID string) (GroupMember, error)
	ListMembers(ctx context.Context, groupID string) ([]GroupMember, error)
	AddMember(ctx context.Context, groupID string, in AddMemberInput) (GroupMember, error)
	UpdateMember(ctx context.Context, groupID, userID string, in UpdateMemberInput) (GroupMember, error)
	RemoveMember(ctx context.Context, groupID, userID string) error
}
