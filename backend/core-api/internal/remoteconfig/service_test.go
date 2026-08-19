package remoteconfig_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	"github.com/example/core-platform/backend/core-api/internal/remoteconfig/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

func newService(admins map[string]bool) *remoteconfig.Service {
	return remoteconfig.NewService(memory.New(), fakeAdmin{admins: admins})
}

func TestSetRequiresAdmin(t *testing.T) {
	svc := newService(nil)
	_, err := svc.Set(context.Background(), "u1", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: 1})
	if !errors.Is(err, remoteconfig.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSetValidatesRequiredFields(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	_, err := svc.Set(context.Background(), "admin", remoteconfig.SetInput{})
	var verr *remoteconfig.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestGetIsOpenToAnyAuthenticatedCaller(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "checkout.maxRetries", Value: float64(3)}); err != nil {
		t.Fatalf("set: %v", err)
	}
	entry, err := svc.Get(ctx, "app-1", "production", "checkout.maxRetries")
	if err != nil {
		t.Fatalf("expected a non-admin read to succeed, got %v", err)
	}
	if entry.Value != float64(3) {
		t.Fatalf("expected value 3, got %v", entry.Value)
	}
}

func TestSetTwiceUpdatesInPlaceAndTracksPreviousValue(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: "v1"}); err != nil {
		t.Fatalf("set 1: %v", err)
	}
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: "v2"}); err != nil {
		t.Fatalf("set 2: %v", err)
	}
	entry, err := svc.Get(ctx, "app-1", "production", "x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if entry.Value != "v2" {
		t.Fatalf("expected the latest value v2, got %v", entry.Value)
	}

	history, err := svc.History(ctx, "admin", "app-1", "production", "x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	// Newest first.
	if history[0].NewValue != "v2" || history[0].PreviousValue != "v1" {
		t.Fatalf("unexpected most recent change: %+v", history[0])
	}
	if history[1].NewValue != "v1" || history[1].PreviousValue != nil {
		t.Fatalf("unexpected first-ever change: %+v", history[1])
	}
}

func TestEnvironmentsAreIndependent(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: "prod-value"}); err != nil {
		t.Fatalf("set production: %v", err)
	}
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "staging", Key: "x", Value: "staging-value"}); err != nil {
		t.Fatalf("set staging: %v", err)
	}
	prod, err := svc.Get(ctx, "app-1", "production", "x")
	if err != nil || prod.Value != "prod-value" {
		t.Fatalf("expected prod-value, got %v err=%v", prod.Value, err)
	}
	staging, err := svc.Get(ctx, "app-1", "staging", "x")
	if err != nil || staging.Value != "staging-value" {
		t.Fatalf("expected staging-value, got %v err=%v", staging.Value, err)
	}
}

func TestDeleteRequiresAdminAndRecordsHistory(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: "v1"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := svc.Delete(ctx, "u1", "app-1", "production", "x", "cleanup"); !errors.Is(err, remoteconfig.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	if err := svc.Delete(ctx, "admin", "app-1", "production", "x", "cleanup"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, "app-1", "production", "x"); !errors.Is(err, remoteconfig.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	history, err := svc.History(ctx, "admin", "app-1", "production", "x")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 || history[0].NewValue != nil || history[0].Reason != "cleanup" {
		t.Fatalf("expected the delete to appear as a history entry with NewValue nil, got %+v", history)
	}
}

func TestHistoryRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "x", Value: "v1"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.History(ctx, "u1", "app-1", "production", "x"); !errors.Is(err, remoteconfig.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestListScopedToAppAndEnvironment(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "a", Value: 1}); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "production", Key: "b", Value: 2}); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if _, err := svc.Set(ctx, "admin", remoteconfig.SetInput{AppID: "app-1", Environment: "staging", Key: "a", Value: 3}); err != nil {
		t.Fatalf("set staging a: %v", err)
	}
	list, err := svc.List(ctx, "app-1", "production")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 production entries, got %d", len(list))
	}
}
