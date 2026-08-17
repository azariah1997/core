package identity_test

import (
	"context"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/identity/memory"
)

func newService() *identity.Service {
	return identity.NewService("fake", memory.Provider{}, memory.New())
}

func TestAuthenticateCreatesIdentityOnFirstSight(t *testing.T) {
	svc := newService()
	id, err := svc.Authenticate(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if id.ProviderSubject != "user-123" || id.Provider != "fake" {
		t.Fatalf("unexpected identity: %+v", id)
	}
	if id.Status != identity.StatusActive {
		t.Fatalf("expected new identity to default to active, got %s", id.Status)
	}
	if id.LastLoginAt == nil {
		t.Fatal("expected LastLoginAt to be set")
	}
}

func TestAuthenticateReusesExistingIdentity(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	first, err := svc.Authenticate(ctx, "user-123")
	if err != nil {
		t.Fatalf("first authenticate: %v", err)
	}
	second, err := svc.Authenticate(ctx, "user-123")
	if err != nil {
		t.Fatalf("second authenticate: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same identity record, got %s and %s", first.ID, second.ID)
	}
}

func TestAuthenticateRejectsInvalidToken(t *testing.T) {
	svc := newService()
	_, err := svc.Authenticate(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error for an empty token")
	}
}

func TestDisableChangesStatus(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	id, err := svc.Authenticate(ctx, "user-123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if err := svc.Disable(ctx, id.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
}
