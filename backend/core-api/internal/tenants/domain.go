// Package tenants implements the platform's reusable multi-tenancy
// boundary. Consumer apps can ignore it entirely and operate against a
// single implicit tenant; SaaS applications create many. Every tenant
// belongs to exactly one Application (Phase 2) - the platform never
// assumes all future applications are consumer-only single-tenant ones.
package tenants

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

func (s Status) valid() bool {
	switch s {
	case StatusActive, StatusSuspended:
		return true
	default:
		return false
	}
}

type MembershipRole string

const (
	RoleOwner  MembershipRole = "owner"
	RoleAdmin  MembershipRole = "admin"
	RoleMember MembershipRole = "member"
)

func (r MembershipRole) valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

// canManage reports whether this role may change tenant settings or
// membership - owner and admin, not plain member.
func (r MembershipRole) canManage() bool {
	return r == RoleOwner || r == RoleAdmin
}

type Tenant struct {
	ID        string
	AppID     string
	Slug      string
	Name      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Membership struct {
	ID        string
	TenantID  string
	UserID    string
	Role      MembershipRole
	CreatedAt time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound           = errors.New("tenant not found")
	ErrSlugTaken          = errors.New("slug already in use")
	ErrMembershipNotFound = errors.New("membership not found")
	ErrAlreadyMember      = errors.New("user is already a member")
	// ErrForbidden and ErrManagerRequired are the two membership-based
	// authorization failures every write/read here can hit - deliberately
	// answered from Membership data directly rather than routed through
	// authz.Service: "is this user an owner/admin of this specific
	// tenant" is this package's own data, not a cross-domain question.
	ErrForbidden       = errors.New("not a member of this tenant")
	ErrManagerRequired = errors.New("owner or admin role required")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type CreateInput struct {
	AppID string
	Slug  string
	Name  string
}

func (in CreateInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Slug) == "" || !slugPattern.MatchString(in.Slug):
		return &ValidationError{"slug must be lowercase alphanumeric with optional internal hyphens"}
	case strings.TrimSpace(in.Name) == "":
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
		return &ValidationError{"status must be one of active, suspended"}
	}
	return nil
}

type AddMemberInput struct {
	UserID string
	Role   MembershipRole
}

func (in AddMemberInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return &ValidationError{"userId is required"}
	}
	if !in.Role.valid() {
		return &ValidationError{"role must be one of owner, admin, member"}
	}
	return nil
}

// Repository is the storage-agnostic boundary.
type Repository interface {
	// Create creates the tenant and an owner Membership for ownerUserID
	// atomically - a tenant can never transiently exist with no owner.
	Create(ctx context.Context, ownerUserID string, in CreateInput) (Tenant, error)
	Get(ctx context.Context, id string) (Tenant, error)
	ListForUser(ctx context.Context, userID string) ([]Tenant, error)
	Update(ctx context.Context, id string, in UpdateInput) (Tenant, error)

	GetMembership(ctx context.Context, tenantID, userID string) (Membership, error)
	ListMembers(ctx context.Context, tenantID string) ([]Membership, error)
	AddMember(ctx context.Context, tenantID string, in AddMemberInput) (Membership, error)
	RemoveMember(ctx context.Context, tenantID, userID string) error
}
