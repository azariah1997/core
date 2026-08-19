package authz

import (
	"context"
	"log/slog"
)

// platformResource is the one singleton object platform.admin's fine-grained
// grant is written against - "does this user have admin on the platform as
// a whole", not tied to any specific resource instance.
var platformResource = Resource{Type: "platform", ID: "core"}

const actionAdmin Action = "admin"

// AuditRecorder is a narrow, purpose-built interface using only
// primitive types - not audit.RecordInput - so this package never needs
// to import internal/audit, the same consumer-defined-interface pattern
// as notifications.AdminChecker/features.AdminChecker (satisfied there
// directly by *authz.Service; here the direction is reversed, satisfied
// by a small adapter over *audit.Service - see
// internal/api/audit_adapter.go).
type AuditRecorder interface {
	RecordRoleChange(ctx context.Context, actorUserID, action, targetUserID, role string) error
}

type Service struct {
	roles    RoleRepository
	provider Provider
	audit    AuditRecorder
	logger   *slog.Logger
}

func NewService(roles RoleRepository, provider Provider, audit AuditRecorder, logger *slog.Logger) *Service {
	return &Service{roles: roles, provider: provider, audit: audit, logger: logger}
}

func (s *Service) HasRole(ctx context.Context, userID string, role Role) (bool, error) {
	roles, err := s.roles.RolesFor(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

// AssignRole is the one place a role is granted - for platform.admin it
// also grants the equivalent fine-grained relationship in Provider, so
// Can() sees it too. Postgres is written first: if the OpenFGA write then
// fails, the role still exists and AssignRole can simply be retried: it's
// naturally idempotent, so partial failure here self-heals rather than
// leaving two systems permanently disagreeing.
//
// actingUserID is who performed the grant - a privilege change is
// exactly the kind of sensitive, compliance-relevant action Phase 19's
// Audit Service exists for, so a successful grant records one. The audit
// write is best-effort: a failure to record it is logged, not returned,
// so an audit-service hiccup can never block a legitimate role change -
// the same "never let a best-effort side effect fail the primary
// operation" principle messaging/notifications already apply to realtime
// pushes.
func (s *Service) AssignRole(ctx context.Context, actingUserID, userID string, role Role) error {
	if err := s.roles.AssignRole(ctx, userID, role); err != nil {
		return err
	}
	if role == RolePlatformAdmin {
		if err := s.provider.Grant(ctx, userID, actionAdmin, platformResource); err != nil {
			return err
		}
	}
	if err := s.audit.RecordRoleChange(ctx, actingUserID, "role.assigned", userID, string(role)); err != nil {
		s.logger.Error("authz: failed to record audit event for role assignment", "error", err, "actorUserId", actingUserID, "targetUserId", userID, "role", role)
	}
	return nil
}

func (s *Service) RevokeRole(ctx context.Context, actingUserID, userID string, role Role) error {
	if err := s.roles.RevokeRole(ctx, userID, role); err != nil {
		return err
	}
	if role == RolePlatformAdmin {
		if err := s.provider.Revoke(ctx, userID, actionAdmin, platformResource); err != nil {
			return err
		}
	}
	if err := s.audit.RecordRoleChange(ctx, actingUserID, "role.revoked", userID, string(role)); err != nil {
		s.logger.Error("authz: failed to record audit event for role revocation", "error", err, "actorUserId", actingUserID, "targetUserId", userID, "role", role)
	}
	return nil
}

// Can is the single entry point every domain module must use instead of
// implementing permission logic independently.
func (s *Service) Can(ctx context.Context, subjectUserID string, action Action, resource Resource) (bool, error) {
	return s.provider.Can(ctx, subjectUserID, action, resource)
}

// IsPlatformAdmin is a convenience for the extremely common "does this
// caller have blanket admin access" check, backed by the same fine-grained
// grant AssignRole writes - so it stays consistent with Can() rather than
// being a separate, potentially-drifting code path.
func (s *Service) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.Can(ctx, userID, actionAdmin, platformResource)
}
