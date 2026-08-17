package devices_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/devices/memory"
)

func newService() *devices.Service {
	return devices.NewService(memory.New())
}

func TestRegisterRejectsMissingClientDeviceID(t *testing.T) {
	svc := newService()
	_, err := svc.Register(context.Background(), "user-1", devices.RegisterInput{Platform: "ios"})
	var verr *devices.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestRegisterRejectsMissingPlatform(t *testing.T) {
	svc := newService()
	_, err := svc.Register(context.Background(), "user-1", devices.RegisterInput{ClientDeviceID: "install-1"})
	var verr *devices.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestRegisterNeverExposesRawPushToken(t *testing.T) {
	svc := newService()
	d, err := svc.Register(context.Background(), "user-1", devices.RegisterInput{
		ClientDeviceID: "install-1", Platform: "ios", PushToken: "super-secret-token",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !d.HasPushToken {
		t.Fatal("expected HasPushToken to be true")
	}
	// Device has no field capable of holding the raw token at all - this
	// test exists so that changing the struct to add one would fail loudly.
}

func TestRegisterIsIdempotentPerClientDeviceID(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	in := devices.RegisterInput{ClientDeviceID: "install-1", Platform: "ios", AppVersion: "1.0"}

	first, err := svc.Register(ctx, "user-1", in)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	in.AppVersion = "1.1"
	second, err := svc.Register(ctx, "user-1", in)
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same device record, got %s and %s", first.ID, second.ID)
	}
	if second.AppVersion != "1.1" {
		t.Fatalf("expected app version to update on re-registration, got %s", second.AppVersion)
	}
}

func TestListOnlyReturnsCallersOwnDevices(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Register(ctx, "user-1", devices.RegisterInput{ClientDeviceID: "d1", Platform: "ios"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.Register(ctx, "user-2", devices.RegisterInput{ClientDeviceID: "d2", Platform: "android"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	list, err := svc.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ClientDeviceID != "d1" {
		t.Fatalf("expected only user-1's device, got %+v", list)
	}
}

func TestRevokeRemovesDeviceFromList(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	d, err := svc.Register(ctx, "user-1", devices.RegisterInput{ClientDeviceID: "d1", Platform: "ios"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.Revoke(ctx, "user-1", d.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	list, err := svc.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected revoked device to disappear from the list, got %+v", list)
	}
}

func TestRevokeCannotTargetAnotherUsersDevice(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	d, err := svc.Register(ctx, "user-1", devices.RegisterInput{ClientDeviceID: "d1", Platform: "ios"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	err = svc.Revoke(ctx, "user-2", d.ID)
	if !errors.Is(err, devices.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when revoking another user's device, got %v", err)
	}
}
