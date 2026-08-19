package api

import (
	"context"

	"github.com/example/core-platform/backend/core-api/internal/audit"
)

// RoleChangeAuditRecorder implements authz.AuditRecorder using
// audit.Service. The translation from authz's narrow, primitive-typed
// interface into audit.RecordInput lives here, not in authz itself,
// keeping authz decoupled from audit's concrete types - the same
// adapter pattern as profileAccessChecker and devicePushTokenLookup.
//
// audit.Service itself depends on authz.Service (as an AdminChecker),
// which creates a construction-order cycle with authz.Service depending
// on this recorder: neither can be built first. Broken by two-phase
// wiring - cmd/server/main.go constructs this recorder empty, builds
// authz.Service with it, then builds audit.Service using authz.Service
// as its AdminChecker, then calls SetAuditService to complete the
// wiring. Safe because AssignRole/RevokeRole (the only callers of
// RecordRoleChange) are never invoked during startup, only from later
// API requests - by then SetAuditService has always already run.
type RoleChangeAuditRecorder struct {
	auditSvc *audit.Service
}

func NewRoleChangeAuditRecorder() *RoleChangeAuditRecorder {
	return &RoleChangeAuditRecorder{}
}

func (r *RoleChangeAuditRecorder) SetAuditService(s *audit.Service) {
	r.auditSvc = s
}

func (r *RoleChangeAuditRecorder) RecordRoleChange(ctx context.Context, actorUserID, action, targetUserID, role string) error {
	_, err := r.auditSvc.Record(ctx, audit.RecordInput{
		ActorUserID: actorUserID, Action: action, ResourceType: "user", ResourceID: targetUserID,
		Metadata: map[string]any{"role": role},
	})
	return err
}
