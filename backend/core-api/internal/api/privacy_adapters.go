package api

import (
	"context"
	"errors"

	"github.com/example/core-platform/backend/core-api/internal/audit"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/files"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

// Phase 20's Exporter/Deleter adapters. Each wraps an existing domain
// Service's already-public methods (privacy never reaches into another
// module's Repository or storage directly) - the same "consumer defines
// the interface, a small adapter here satisfies it" pattern as every
// other cross-module wiring in this repo.
//
// Only four modules participate for real in this phase: users, devices,
// files, and audit (export-only - see auditPrivacyParticipant below).
// Each demonstrates a different data shape (profile fields, a relational
// list, object storage, an immutable append-only log). Deliberately
// deferred, with real reasons, not laziness:
//   - relationships/groups/messaging: shared, multi-user data - deleting
//     a relationship or a message outright would corrupt the OTHER
//     participant's view of their own history. That needs a redaction/
//     anonymization strategy more nuanced than this phase's registry,
//     which assumes a participant can act on "this user's data" in
//     isolation. A real follow-up, not solved here.
//   - notifications: this registry is what makes adding it later a
//     small, isolated change (one adapter, two lines in main.go) rather
//     than a framework change - proving that is more valuable than
//     wiring every module up front.
//   - tenants/jobs/workflows/features/remoteconfig: operational or
//     administrative data about actions a user took, not personal data
//     about the user, the same distinction GDPR itself draws.

// usersPrivacyParticipant exports/deletes the platform profile itself -
// DeleteUserData calls the same Service.Delete Phase 4 already built
// (soft delete: the row survives for referential integrity, but the
// user is gone from every normal API's perspective) rather than a
// separate hard-delete path.
type usersPrivacyParticipant struct{ svc *users.Service }

func NewUsersPrivacyParticipant(svc *users.Service) *usersPrivacyParticipant {
	return &usersPrivacyParticipant{svc: svc}
}

func (p *usersPrivacyParticipant) ExportUserData(ctx context.Context, userID string) (map[string]any, error) {
	u, err := p.svc.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) || errors.Is(err, users.ErrDeleted) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	return map[string]any{
		"displayName": u.DisplayName, "avatarRef": u.AvatarRef, "locale": u.Locale, "timezone": u.Timezone,
		"status": u.Status, "createdAt": u.CreatedAt, "updatedAt": u.UpdatedAt,
	}, nil
}

func (p *usersPrivacyParticipant) DeleteUserData(ctx context.Context, userID string) error {
	_, err := p.svc.Delete(ctx, userID)
	return err
}

// devicesPrivacyParticipant never exports raw push tokens - HasPushToken
// only, the same rule Phase 5's own HTTP responses already follow.
type devicesPrivacyParticipant struct{ svc *devices.Service }

func NewDevicesPrivacyParticipant(svc *devices.Service) *devicesPrivacyParticipant {
	return &devicesPrivacyParticipant{svc: svc}
}

func (p *devicesPrivacyParticipant) ExportUserData(ctx context.Context, userID string) (map[string]any, error) {
	list, err := p.svc.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(list))
	for _, d := range list {
		items = append(items, map[string]any{
			"platform": d.Platform, "osVersion": d.OSVersion, "appVersion": d.AppVersion,
			"hasPushToken": d.HasPushToken, "sessionStatus": d.SessionStatus, "lastActiveAt": d.LastActiveAt,
		})
	}
	return map[string]any{"devices": items}, nil
}

func (p *devicesPrivacyParticipant) DeleteUserData(ctx context.Context, userID string) error {
	return p.svc.RevokeAll(ctx, userID)
}

// filesPrivacyParticipant exports metadata only, never file bytes
// (matching Phase 13's own "the client talks to storage directly"
// principle - a real export UI would surface each file's normal
// presigned download URL, not a copy of the bytes duplicated into the
// export bundle itself), and deletes both the object and the row via
// files.Service.DeleteAllForUser.
type filesPrivacyParticipant struct{ svc *files.Service }

func NewFilesPrivacyParticipant(svc *files.Service) *filesPrivacyParticipant {
	return &filesPrivacyParticipant{svc: svc}
}

func (p *filesPrivacyParticipant) ExportUserData(ctx context.Context, userID string) (map[string]any, error) {
	var items []map[string]any
	cursor := ""
	for {
		result, err := p.svc.ListMine(ctx, userID, files.ListParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, f := range result.Items {
			items = append(items, map[string]any{
				"fileName": f.FileName, "mimeType": f.MimeType, "byteSize": f.ByteSize,
				"visibility": f.Visibility, "status": f.Status, "createdAt": f.CreatedAt,
			})
		}
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return map[string]any{"files": items}, nil
}

func (p *filesPrivacyParticipant) DeleteUserData(ctx context.Context, userID string) error {
	return p.svc.DeleteAllForUser(ctx, userID)
}

// auditPrivacyParticipant is Exporter-only - deliberately does not
// implement privacy.Deleter at all. Audit records are immutable by
// design (Phase 19: no Update or Delete method exists anywhere in
// audit's own API, reinforced by a database trigger) - a user's own
// past actions are exportable (they're entitled to see what was
// recorded about them) but not erasable, even by this phase's own
// deletion workflow. The type system enforces this: privacy.Service.
// RunDeletion only ever iterates registered Deleters, and audit was
// never registered as one - see cmd/server/main.go.
type auditPrivacyParticipant struct{ svc *audit.Service }

func NewAuditPrivacyParticipant(svc *audit.Service) *auditPrivacyParticipant {
	return &auditPrivacyParticipant{svc: svc}
}

func (p *auditPrivacyParticipant) ExportUserData(ctx context.Context, userID string) (map[string]any, error) {
	result, err := p.svc.ListMine(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(result.Items))
	for _, rec := range result.Items {
		items = append(items, map[string]any{
			"action": rec.Action, "resourceType": rec.ResourceType, "resourceId": rec.ResourceID,
			"occurredAt": rec.OccurredAt, "metadata": rec.Metadata,
		})
	}
	return map[string]any{"auditEvents": items}, nil
}
