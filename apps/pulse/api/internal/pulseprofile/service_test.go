package pulseprofile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile/memory"
)

func newService() *pulseprofile.Service {
	return pulseprofile.NewService(memory.New())
}

func TestEnsureProfileCreatesOnFirstCall(t *testing.T) {
	svc := newService()
	p, err := svc.EnsureProfile(context.Background(), "user-1", pulseprofile.CreateInput{Handle: "rachel"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if p.UserID != "user-1" || p.Handle != "rachel" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestEnsureProfileIsIdempotentForTheSameCaller(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	first, err := svc.EnsureProfile(ctx, "user-1", pulseprofile.CreateInput{Handle: "rachel"})
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := svc.EnsureProfile(ctx, "user-1", pulseprofile.CreateInput{Handle: "ignored-on-second-call"})
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.Handle != first.Handle {
		t.Fatalf("expected the existing profile to be returned unchanged, got %+v", second)
	}
}

func TestEnsureProfileRejectsAnInvalidHandle(t *testing.T) {
	svc := newService()
	_, err := svc.EnsureProfile(context.Background(), "user-1", pulseprofile.CreateInput{Handle: "a"})
	var verr *pulseprofile.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestEnsureProfileRejectsADuplicateHandleAcrossDifferentCallers(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.EnsureProfile(ctx, "user-1", pulseprofile.CreateInput{Handle: "rachel"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := svc.EnsureProfile(ctx, "user-2", pulseprofile.CreateInput{Handle: "rachel"})
	if !errors.Is(err, pulseprofile.ErrHandleTaken) {
		t.Fatalf("expected ErrHandleTaken, got %v", err)
	}
}

func TestGetByHandleIsCaseInsensitive(t *testing.T) {
	svc := newService()
	if _, err := svc.EnsureProfile(context.Background(), "user-1", pulseprofile.CreateInput{Handle: "rachel"}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	p, err := svc.GetByHandle(context.Background(), "RACHEL")
	if err != nil {
		t.Fatalf("get by handle: %v", err)
	}
	if p.UserID != "user-1" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestUpdateMergesPreferencesWithoutClobberingUnsetFields(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.EnsureProfile(ctx, "user-1", pulseprofile.CreateInput{Handle: "rachel"}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := svc.Update(ctx, "user-1", pulseprofile.UpdateInput{VisualPrefs: map[string]any{"theme": "dark"}}); err != nil {
		t.Fatalf("update visual: %v", err)
	}
	p, err := svc.Update(ctx, "user-1", pulseprofile.UpdateInput{PulsePrefs: map[string]any{"defaultDurationMs": float64(2000)}})
	if err != nil {
		t.Fatalf("update pulse prefs: %v", err)
	}
	if p.VisualPrefs["theme"] != "dark" {
		t.Fatalf("expected the earlier visual prefs update to survive, got %+v", p.VisualPrefs)
	}
	if p.PulsePrefs["defaultDurationMs"] != float64(2000) {
		t.Fatalf("unexpected pulse prefs: %+v", p.PulsePrefs)
	}
}

func TestGetReturnsNotFoundForAnUnknownCaller(t *testing.T) {
	svc := newService()
	_, err := svc.Get(context.Background(), "does-not-exist")
	if !errors.Is(err, pulseprofile.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
