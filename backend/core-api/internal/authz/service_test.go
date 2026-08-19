package authz_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/authz/memory"
)

// fakeAuditRecorder discards audit events - these tests exercise role
// assignment/revocation semantics, not audit recording (that's audit's
// own package tests).
type fakeAuditRecorder struct{}

func (fakeAuditRecorder) RecordRoleChange(ctx context.Context, actorUserID, action, targetUserID, role string) error {
	return nil
}

func newService() *authz.Service {
	return authz.NewService(memory.NewRoleRepository(), memory.NewProvider(), fakeAuditRecorder{}, slog.Default())
}

func TestHasRoleFalseBeforeAssignment(t *testing.T) {
	svc := newService()
	has, err := svc.HasRole(context.Background(), "user-1", authz.RolePlatformAdmin)
	if err != nil {
		t.Fatalf("has role: %v", err)
	}
	if has {
		t.Fatal("expected no role before assignment")
	}
}

func TestAssignRoleThenHasRole(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RoleSupport); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	has, err := svc.HasRole(ctx, "user-1", authz.RoleSupport)
	if err != nil {
		t.Fatalf("has role: %v", err)
	}
	if !has {
		t.Fatal("expected role after assignment")
	}
}

func TestRevokeRoleRemovesIt(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RoleModerator); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := svc.RevokeRole(ctx, "user-1", "user-1", authz.RoleModerator); err != nil {
		t.Fatalf("revoke role: %v", err)
	}
	has, err := svc.HasRole(ctx, "user-1", authz.RoleModerator)
	if err != nil {
		t.Fatalf("has role: %v", err)
	}
	if has {
		t.Fatal("expected role to be gone after revoke")
	}
}

func TestAssigningPlatformAdminGrantsFineGrainedAccess(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	isAdmin, err := svc.IsPlatformAdmin(ctx, "user-1")
	if err != nil {
		t.Fatalf("is platform admin: %v", err)
	}
	if !isAdmin {
		t.Fatal("expected platform.admin role assignment to also grant the fine-grained admin relation")
	}
}

func TestRevokingPlatformAdminRemovesFineGrainedAccess(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := svc.RevokeRole(ctx, "user-1", "user-1", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("revoke role: %v", err)
	}
	isAdmin, err := svc.IsPlatformAdmin(ctx, "user-1")
	if err != nil {
		t.Fatalf("is platform admin: %v", err)
	}
	if isAdmin {
		t.Fatal("expected fine-grained admin relation to be revoked alongside the role")
	}
}

func TestCanIsFalseByDefault(t *testing.T) {
	svc := newService()
	allowed, err := svc.Can(context.Background(), "user-1", "view", authz.Resource{Type: "user_profile", ID: "user-2"})
	if err != nil {
		t.Fatalf("can: %v", err)
	}
	if allowed {
		t.Fatal("expected no access without an explicit grant")
	}
}

func TestOtherUsersRolesAreUnaffected(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	isAdmin, err := svc.IsPlatformAdmin(ctx, "user-2")
	if err != nil {
		t.Fatalf("is platform admin: %v", err)
	}
	if isAdmin {
		t.Fatal("expected a different user's admin status to be unaffected")
	}
}

func TestRolesForAllowsSelf(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "user-1", "user-1", authz.RoleSupport); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	roles, err := svc.RolesFor(ctx, "user-1", "user-1")
	if err != nil {
		t.Fatalf("expected a user to view their own roles, got %v", err)
	}
	if len(roles) != 1 || roles[0] != authz.RoleSupport {
		t.Fatalf("unexpected roles: %v", roles)
	}
}

func TestRolesForRequiresAdminForOthers(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if err := svc.AssignRole(ctx, "admin", "admin", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("assign admin: %v", err)
	}
	if err := svc.AssignRole(ctx, "admin", "user-1", authz.RoleModerator); err != nil {
		t.Fatalf("assign role to user-1: %v", err)
	}
	if _, err := svc.RolesFor(ctx, "stranger", "user-1"); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger, got %v", err)
	}
	roles, err := svc.RolesFor(ctx, "admin", "user-1")
	if err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
	if len(roles) != 1 || roles[0] != authz.RoleModerator {
		t.Fatalf("unexpected roles: %v", roles)
	}
}
