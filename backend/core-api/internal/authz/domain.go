// Package authz is the platform's single authorization service: RBAC roles
// plus a fine-grained resource/relationship permission engine (OpenFGA).
// Other domain modules depend on this instead of implementing permission
// logic themselves, per the platform's non-negotiable rule.
package authz

import "context"

// Role is a coarse, platform-wide capability. Role assignment is the
// platform's own data (RoleRepository), never delegated to Provider - an
// external engine shouldn't own who counts as an admin.
type Role string

const (
	RolePlatformAdmin Role = "platform.admin"
	RoleSupport       Role = "support"
	RoleModerator     Role = "moderator"
)

// Action and Resource describe a fine-grained permission question:
// "may subject <action> <resource>?"
type Action string

type Resource struct {
	Type string
	ID   string
}

// Provider is the fine-grained resource/relationship permission engine -
// OpenFGA today, swappable later without touching callers.
type Provider interface {
	Can(ctx context.Context, subjectUserID string, action Action, resource Resource) (bool, error)
	Grant(ctx context.Context, subjectUserID string, action Action, resource Resource) error
	Revoke(ctx context.Context, subjectUserID string, action Action, resource Resource) error
}

// RoleRepository is the storage-agnostic boundary for RBAC role
// assignment.
type RoleRepository interface {
	RolesFor(ctx context.Context, userID string) ([]Role, error)
	AssignRole(ctx context.Context, userID string, role Role) error
	RevokeRole(ctx context.Context, userID string, role Role) error
}
