package users_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/backend/core-api/internal/users/memory"
)

func newService() *users.Service {
	return users.NewService(memory.New())
}

func TestCreateRejectsEmptyDisplayName(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "  "})
	var verr *users.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateDefaultsLocaleAndTimezone(t *testing.T) {
	svc := newService()
	u, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Locale != "en-GB" || u.Timezone != "UTC" {
		t.Fatalf("expected default locale/timezone, got %q/%q", u.Locale, u.Timezone)
	}
	if u.Status != users.StatusActive {
		t.Fatalf("expected new user to default to active, got %s", u.Status)
	}
}

func TestUpdateRequiresAtLeastOneField(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.Update(ctx, u.ID, users.UpdateInput{})
	var verr *users.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestUpdateRejectsDeletedStatus(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	deleted := users.StatusDeleted
	_, err = svc.Update(ctx, u.ID, users.UpdateInput{Status: &deleted})
	var verr *users.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError rejecting deleted via Update, got %v", err)
	}
}

func TestUpdateChangesFields(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newName := "Ada Lovelace"
	deactivated := users.StatusDeactivated
	updated, err := svc.Update(ctx, u.ID, users.UpdateInput{DisplayName: &newName, Status: &deactivated})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != "Ada Lovelace" || updated.Status != users.StatusDeactivated {
		t.Fatalf("unexpected user after update: %+v", updated)
	}
}

func TestDeleteThenGetReturnsNotFound(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = svc.Get(ctx, u.ID)
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteThenUpdateReturnsNotFound(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	newName := "Someone Else"
	_, err = svc.Update(ctx, u.ID, users.UpdateInput{DisplayName: &newName})
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating a deleted user, got %v", err)
	}
}

type fakeLinker struct {
	linked map[string]string
}

func (f *fakeLinker) LinkUser(ctx context.Context, identityID, userID string) error {
	if f.linked == nil {
		f.linked = map[string]string{}
	}
	f.linked[identityID] = userID
	return nil
}

func TestEnsureForIdentityCreatesAndLinksOnFirstLogin(t *testing.T) {
	svc := newService()
	linker := &fakeLinker{}
	u, err := svc.EnsureForIdentity(context.Background(), linker, "identity-1", nil, "ada")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if u.DisplayName != "ada" {
		t.Fatalf("expected display name derived from claims, got %q", u.DisplayName)
	}
	if linker.linked["identity-1"] != u.ID {
		t.Fatalf("expected identity to be linked to the new user")
	}
}

func TestEnsureForIdentityFallsBackToGenericNameWhenClaimsEmpty(t *testing.T) {
	svc := newService()
	linker := &fakeLinker{}
	u, err := svc.EnsureForIdentity(context.Background(), linker, "identity-1", nil, "")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if u.DisplayName == "" {
		t.Fatal("expected a non-empty fallback display name")
	}
}

func TestEnsureForIdentityReusesLinkedUser(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	existing, err := svc.Create(ctx, users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	linker := &fakeLinker{}
	resolved, err := svc.EnsureForIdentity(ctx, linker, "identity-1", &existing.ID, "ignored")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if resolved.ID != existing.ID {
		t.Fatalf("expected to reuse the existing linked user, got a different id")
	}
	if len(linker.linked) != 0 {
		t.Fatal("expected no new link to be created when already linked")
	}
}
