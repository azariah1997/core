package groups_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/groups"
	"github.com/example/core-platform/backend/core-api/internal/groups/memory"
)

func newService() *groups.Service {
	return groups.NewService(memory.New())
}

func TestCreateRejectsEmptyName(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), "creator", groups.CreateInput{AppID: "app-1", Name: "  "})
	var verr *groups.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateMakesCreatorAManager(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Family"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(ctx, "creator", g.ID); err != nil {
		t.Fatalf("get as creator: %v", err)
	}
	members, err := svc.ListMembers(ctx, "creator", g.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || !members[0].IsManager {
		t.Fatalf("expected creator to be a manager, got %+v", members)
	}
}

func TestNonMemberCannotGet(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Family"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.Get(ctx, "stranger", g.ID)
	if !errors.Is(err, groups.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestPlainMemberCannotUpdateOrAddMembers(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Guild"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "recruit", Role: "recruit"}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	newName := "Renamed"
	if _, err := svc.Update(ctx, "recruit", g.ID, groups.UpdateInput{Name: &newName}); !errors.Is(err, groups.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired for update, got %v", err)
	}
	if _, err := svc.AddMember(ctx, "recruit", g.ID, groups.AddMemberInput{UserID: "another", Role: "recruit"}); !errors.Is(err, groups.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired for add member, got %v", err)
	}
}

func TestPlainMemberCannotPromoteThemselves(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Guild"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "recruit"}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	promote := true
	_, err = svc.UpdateMember(ctx, "recruit", g.ID, "recruit", groups.UpdateMemberInput{IsManager: &promote})
	if !errors.Is(err, groups.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired, got %v", err)
	}
}

func TestManagerCanPromoteAnotherMember(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Guild"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "officer"}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	promote := true
	role := "officer"
	updated, err := svc.UpdateMember(ctx, "creator", g.ID, "officer", groups.UpdateMemberInput{IsManager: &promote, Role: &role})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !updated.IsManager || updated.Role != "officer" {
		t.Fatalf("unexpected member after promotion: %+v", updated)
	}

	// The newly promoted manager can now do manager-only things.
	if _, err := svc.AddMember(ctx, "officer", g.ID, groups.AddMemberInput{UserID: "recruit"}); err != nil {
		t.Fatalf("expected newly promoted manager to add members, got %v", err)
	}
}

func TestFreeFormRoleLabelIsNeverValidated(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Family"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Roles are product-defined free text - "parent" is just as valid as
	// "owner" or "admin"; the platform never enforces a fixed role enum.
	m, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "kid", Role: "child"})
	if err != nil {
		t.Fatalf("add member with arbitrary role label: %v", err)
	}
	if m.Role != "child" {
		t.Fatalf("expected role 'child' to be preserved as-is, got %q", m.Role)
	}
}

func TestMemberCanRemoveThemselves(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Team"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "member-1"}); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := svc.RemoveMember(ctx, "member-1", g.ID, "member-1"); err != nil {
		t.Fatalf("self remove: %v", err)
	}
	if _, err := svc.Get(ctx, "member-1", g.ID); !errors.Is(err, groups.ErrForbidden) {
		t.Fatalf("expected removed member to lose access, got %v", err)
	}
}

func TestPlainMemberCannotRemoveSomeoneElse(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Team"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "m1"}); err != nil {
		t.Fatalf("add m1: %v", err)
	}
	if _, err := svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "m2"}); err != nil {
		t.Fatalf("add m2: %v", err)
	}
	err = svc.RemoveMember(ctx, "m1", g.ID, "m2")
	if !errors.Is(err, groups.ErrManagerRequired) {
		t.Fatalf("expected ErrManagerRequired, got %v", err)
	}
}

func TestAddMemberRejectsDuplicate(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	g, err := svc.Create(ctx, "creator", groups.CreateInput{AppID: "app-1", Name: "Team"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.AddMember(ctx, "creator", g.ID, groups.AddMemberInput{UserID: "creator"})
	if !errors.Is(err, groups.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember re-adding the creator, got %v", err)
	}
}

func TestListMineReturnsOnlyMemberGroups(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "u1", groups.CreateInput{AppID: "app-1", Name: "A"}); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if _, err := svc.Create(ctx, "u2", groups.CreateInput{AppID: "app-1", Name: "B"}); err != nil {
		t.Fatalf("create 2: %v", err)
	}
	mine, err := svc.ListMine(ctx, "u1")
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].Name != "A" {
		t.Fatalf("expected only u1's group, got %+v", mine)
	}
}
