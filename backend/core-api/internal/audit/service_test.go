package audit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/audit"
	"github.com/example/core-platform/backend/core-api/internal/audit/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

func newService(admins map[string]bool) *audit.Service {
	return audit.NewService(memory.New(), fakeAdmin{admins: admins})
}

func TestRecordValidatesRequiredFields(t *testing.T) {
	svc := newService(nil)
	_, err := svc.Record(context.Background(), audit.RecordInput{})
	var verr *audit.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestRecordIsOpenToAnyAuthenticatedCaller(t *testing.T) {
	svc := newService(nil)
	rec, err := svc.Record(context.Background(), audit.RecordInput{
		ActorUserID: "u1", Action: "profile.updated", ResourceType: "user", ResourceID: "u1",
	})
	if err != nil {
		t.Fatalf("expected a non-admin to record an audit event, got %v", err)
	}
	if rec.ID == "" || rec.OccurredAt.IsZero() {
		t.Fatalf("expected a populated record, got %+v", rec)
	}
}

func TestGetRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	rec, err := svc.Record(ctx, audit.RecordInput{ActorUserID: "u1", Action: "x", ResourceType: "y", ResourceID: "z"})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := svc.Get(ctx, "u1", rec.ID); !errors.Is(err, audit.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	if _, err := svc.Get(ctx, "admin", rec.ID); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

func TestListRequiresAdminAndFilters(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Record(ctx, audit.RecordInput{ActorUserID: "u1", Action: "role.assigned", ResourceType: "user", ResourceID: "u2"}); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	if _, err := svc.Record(ctx, audit.RecordInput{ActorUserID: "u1", Action: "role.revoked", ResourceType: "user", ResourceID: "u3"}); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	if _, err := svc.List(ctx, "u1", audit.ListFilter{}); !errors.Is(err, audit.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}

	result, err := svc.List(ctx, "admin", audit.ListFilter{Action: "role.assigned"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ResourceID != "u2" {
		t.Fatalf("expected exactly the role.assigned record, got %+v", result.Items)
	}
}

func TestRecordCapturesEveryNamedField(t *testing.T) {
	svc := newService(nil)
	rec, err := svc.Record(context.Background(), audit.RecordInput{
		ActorUserID: "u1", Action: "role.assigned", ResourceType: "user", ResourceID: "u2",
		AppID: "app-1", TenantID: "tenant-1", DeviceID: "device-1", Metadata: map[string]any{"role": "platform.admin"},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if rec.AppID != "app-1" || rec.TenantID != "tenant-1" || rec.DeviceID != "device-1" || rec.Metadata["role"] != "platform.admin" {
		t.Fatalf("expected every field to round-trip, got %+v", rec)
	}
}

// There is deliberately no TestUpdate/TestDelete here - Service has no
// such methods to call, which is the point.
