package api

import (
	"context"

	"github.com/example/core-platform/backend/core-api/internal/audit"
)

// AIGatewayAuditRecorder implements aigateway.AuditRecorder using
// audit.Service - the same translation-adapter pattern as
// RoleChangeAuditRecorder (audit_adapter.go), but without that one's
// construction-order cycle: audit.Service doesn't depend on aigateway
// for anything, so this can be constructed directly, after
// audit.Service already exists, with no two-phase wiring needed.
type AIGatewayAuditRecorder struct {
	auditSvc *audit.Service
}

func NewAIGatewayAuditRecorder(auditSvc *audit.Service) *AIGatewayAuditRecorder {
	return &AIGatewayAuditRecorder{auditSvc: auditSvc}
}

func (r *AIGatewayAuditRecorder) RecordCompletion(ctx context.Context, actorUserID, action, resourceID string, metadata map[string]any) error {
	_, err := r.auditSvc.Record(ctx, audit.RecordInput{
		ActorUserID: actorUserID, Action: action, ResourceType: "ai_completion", ResourceID: resourceID,
		Metadata: metadata,
	})
	return err
}
