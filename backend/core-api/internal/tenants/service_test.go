package tenants_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/tenants"
	"github.com/example/core-platform/backend/core-api/internal/tenants/memory"
)

func newService() *tenants.Service {
	return tenants.NewService(memory.New())
}

func TestCreateRejectsInvalidSlug(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "Not Valid!", Name: "x"})
	var verr *tenants.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateMakesOwnerAMember(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Owner should be able to Get without a separate grant.
	got, err := svc.Get(ctx, "owner-1", tenant.ID)
	if err != nil {
		t.Fatalf("get as owner: %v", err)
	}
	if got.ID != tenant.ID {
		t.Fatalf("unexpected tenant: %+v", got)
	}
}

func TestNonMemberCannotGet(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.Get(ctx, "stranger", tenant.ID)
	if !errors.Is(err, tenants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestDuplicateSlugConflicts(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "owner-2", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme Again"})
	if !errors.Is(err, tenants.ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestMemberCannotUpdate(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "member-1", Role: tenants.RoleMember}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	newName := "Renamed"
	_, err = svc.Update(ctx, "member-1", tenant.ID, tenants.UpdateInput{Name: &newName})
	if !errors.Is(err, tenants.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired, got %v", err)
	}
}

func TestOwnerCanUpdate(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newName := "Renamed"
	updated, err := svc.Update(ctx, "owner-1", tenant.ID, tenants.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Fatalf("expected renamed tenant, got %+v", updated)
	}
}

func TestAddMemberRequiresManager(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "member-1", Role: tenants.RoleMember}); err != nil {
		t.Fatalf("owner add member: %v", err)
	}
	_, err = svc.AddMember(ctx, "member-1", tenant.ID, tenants.AddMemberInput{UserID: "member-2", Role: tenants.RoleMember})
	if !errors.Is(err, tenants.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired for a plain member adding someone, got %v", err)
	}
}

func TestAddMemberRejectsDuplicate(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "owner-1", Role: tenants.RoleAdmin})
	if !errors.Is(err, tenants.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember re-adding the owner, got %v", err)
	}
}

func TestMemberCanRemoveThemselves(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "member-1", Role: tenants.RoleMember}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := svc.RemoveMember(ctx, "member-1", tenant.ID, "member-1"); err != nil {
		t.Fatalf("self remove: %v", err)
	}
	_, err = svc.Get(ctx, "member-1", tenant.ID)
	if !errors.Is(err, tenants.ErrForbidden) {
		t.Fatalf("expected removed member to lose access, got %v", err)
	}
}

func TestMemberCannotRemoveSomeoneElse(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	tenant, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "member-1", Role: tenants.RoleMember}); err != nil {
		t.Fatalf("add member 1: %v", err)
	}
	if _, err := svc.AddMember(ctx, "owner-1", tenant.ID, tenants.AddMemberInput{UserID: "member-2", Role: tenants.RoleMember}); err != nil {
		t.Fatalf("add member 2: %v", err)
	}
	err = svc.RemoveMember(ctx, "member-1", tenant.ID, "member-2")
	if !errors.Is(err, tenants.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired, got %v", err)
	}
}

func TestListMineReturnsOnlyMemberTenants(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "owner-1", tenants.CreateInput{AppID: "app-1", Slug: "acme", Name: "Acme"}); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if _, err := svc.Create(ctx, "owner-2", tenants.CreateInput{AppID: "app-1", Slug: "globex", Name: "Globex"}); err != nil {
		t.Fatalf("create 2: %v", err)
	}
	mine, err := svc.ListMine(ctx, "owner-1")
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].Slug != "acme" {
		t.Fatalf("expected only owner-1's tenant, got %+v", mine)
	}
}
