package applications_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/backend/core-api/internal/applications/memory"
)

func newService() *applications.Service {
	return applications.NewService(memory.New())
}

func TestCreateRejectsInvalidSlug(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), applications.CreateInput{Slug: "Not Valid!", Name: "x"})
	var verr *applications.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	svc := newService()
	_, err := svc.Create(context.Background(), applications.CreateInput{Slug: "demo", Name: "  "})
	var verr *applications.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateThenGetRoundTrips(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, err := svc.Create(ctx, applications.CreateInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != applications.StatusActive {
		t.Fatalf("expected new application to default to active, got %s", created.Status)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slug != "demo" || got.Name != "Demo" {
		t.Fatalf("unexpected application: %+v", got)
	}
}

func TestCreateDuplicateSlugConflicts(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	if _, err := svc.Create(ctx, applications.CreateInput{Slug: "demo", Name: "Demo"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, applications.CreateInput{Slug: "demo", Name: "Demo Again"})
	if !errors.Is(err, applications.ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

func TestGetRejectsMalformedID(t *testing.T) {
	svc := newService()
	_, err := svc.Get(context.Background(), "not-a-uuid")
	var verr *applications.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestGetUnknownIDNotFound(t *testing.T) {
	svc := newService()
	_, err := svc.Get(context.Background(), "11111111-1111-1111-1111-111111111111")
	if !errors.Is(err, applications.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateRequiresAtLeastOneField(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	created, err := svc.Create(ctx, applications.CreateInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.Update(ctx, created.ID, applications.UpdateInput{})
	var verr *applications.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestUpdateChangesNameAndStatus(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	created, err := svc.Create(ctx, applications.CreateInput{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "Renamed"
	archived := applications.StatusArchived
	updated, err := svc.Update(ctx, created.ID, applications.UpdateInput{Name: &newName, Status: &archived})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Renamed" || updated.Status != applications.StatusArchived {
		t.Fatalf("unexpected application after update: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("expected UpdatedAt to advance")
	}
}

func TestListPaginatesWithCursor(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := svc.Create(ctx, applications.CreateInput{Slug: sluggify(i), Name: sluggify(i)}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	first, err := svc.List(ctx, applications.ListParams{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("expected a first page of 2 with a next cursor, got %+v", first)
	}

	second, err := svc.List(ctx, applications.ListParams{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(second.Items) != 2 {
		t.Fatalf("expected second page of 2, got %d", len(second.Items))
	}
	if first.Items[0].ID == second.Items[0].ID {
		t.Fatal("expected page 2 to contain different items than page 1")
	}
}

func sluggify(i int) string {
	return "app-" + string(rune('a'+i))
}
