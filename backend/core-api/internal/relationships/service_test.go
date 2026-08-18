package relationships_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/relationships"
	"github.com/example/core-platform/backend/core-api/internal/relationships/memory"
)

func newService() *relationships.Service {
	return relationships.NewService(memory.New())
}

func req(appID, requester, target, relType string) relationships.RequestInput {
	return relationships.RequestInput{AppID: appID, RequesterID: requester, TargetID: target, Type: relType}
}

func TestRequestRejectsSelfRelationship(t *testing.T) {
	svc := newService()
	_, err := svc.Request(context.Background(), req("app-1", "u1", "u1", "friend"))
	var verr *relationships.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestRequestCreatesPending(t *testing.T) {
	svc := newService()
	rel, err := svc.Request(context.Background(), req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if rel.Status != relationships.StatusPending {
		t.Fatalf("expected pending, got %s", rel.Status)
	}
}

func TestSecondRequestConflicts(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend")); err != nil {
		t.Fatalf("first request: %v", err)
	}
	_, err := svc.Request(ctx, req("app-1", "u2", "u1", "friend"))
	if !errors.Is(err, relationships.ErrExists) {
		t.Fatalf("expected ErrExists even with reversed direction, got %v", err)
	}
}

func TestRequestRevivesAnEndedRelationship(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	first, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if _, err := svc.Remove(ctx, "u1", first.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	// A new request from the other direction, after the first one ended,
	// should revive the same row rather than being blocked forever.
	revived, err := svc.Request(ctx, req("app-1", "u2", "u1", "friend"))
	if err != nil {
		t.Fatalf("expected revival to succeed, got %v", err)
	}
	if revived.ID != first.ID {
		t.Fatalf("expected the same underlying row to be revived, got a different id")
	}
	if revived.Status != relationships.StatusPending {
		t.Fatalf("expected revived relationship to be pending, got %s", revived.Status)
	}
	if revived.RequesterID != "u2" || revived.TargetID != "u1" {
		t.Fatalf("expected requester/target to reflect the new request, got requester=%s target=%s", revived.RequesterID, revived.TargetID)
	}
}

func TestOnlyTargetCanAccept(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(ctx, "u1", rel.ID); !errors.Is(err, relationships.ErrForbidden) {
		t.Fatalf("expected requester accepting their own request to be forbidden, got %v", err)
	}
	accepted, err := svc.Accept(ctx, "u2", rel.ID)
	if err != nil {
		t.Fatalf("target accept: %v", err)
	}
	if accepted.Status != relationships.StatusActive {
		t.Fatalf("expected active, got %s", accepted.Status)
	}
}

func TestCannotAcceptNonPending(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(ctx, "u2", rel.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	_, err = svc.Accept(ctx, "u2", rel.ID)
	if !errors.Is(err, relationships.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition accepting twice, got %v", err)
	}
}

func TestOnlyTargetCanDecline(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Decline(ctx, "u1", rel.ID); !errors.Is(err, relationships.ErrForbidden) {
		t.Fatalf("expected requester declining to be forbidden, got %v", err)
	}
	declined, err := svc.Decline(ctx, "u2", rel.ID)
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if declined.Status != relationships.StatusEnded {
		t.Fatalf("expected ended, got %s", declined.Status)
	}
}

func TestRequesterCanCancelPending(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Remove(ctx, "u2", rel.ID); !errors.Is(err, relationships.ErrForbidden) {
		t.Fatalf("expected target cancelling a pending request to be forbidden, got %v", err)
	}
	removed, err := svc.Remove(ctx, "u1", rel.ID)
	if err != nil {
		t.Fatalf("requester cancel: %v", err)
	}
	if removed.Status != relationships.StatusEnded {
		t.Fatalf("expected ended, got %s", removed.Status)
	}
}

func TestEitherParticipantCanRemoveActive(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(ctx, "u2", rel.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	removed, err := svc.Remove(ctx, "u2", rel.ID)
	if err != nil {
		t.Fatalf("target remove active: %v", err)
	}
	if removed.Status != relationships.StatusEnded {
		t.Fatalf("expected ended, got %s", removed.Status)
	}
}

func TestStrangerCannotRemove(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(ctx, "u2", rel.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := svc.Remove(ctx, "u3", rel.ID); !errors.Is(err, relationships.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-participant, got %v", err)
	}
}

func TestBlockAStrangerCreatesBlockedRow(t *testing.T) {
	svc := newService()
	rel, err := svc.Block(context.Background(), "u1", "app-1", "u2", "friend")
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if rel.Status != relationships.StatusBlocked {
		t.Fatalf("expected blocked, got %s", rel.Status)
	}
}

func TestBlockTransitionsExistingRelationship(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(ctx, "u2", rel.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	blocked, err := svc.Block(ctx, "u2", "app-1", "u1", "friend")
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if blocked.ID != rel.ID || blocked.Status != relationships.StatusBlocked {
		t.Fatalf("expected the same row transitioned to blocked, got %+v", blocked)
	}
}

func TestBlockedThenRequestConflicts(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Block(ctx, "u1", "app-1", "u2", "friend"); err != nil {
		t.Fatalf("block: %v", err)
	}
	_, err := svc.Request(ctx, req("app-1", "u2", "u1", "friend"))
	if !errors.Is(err, relationships.ErrExists) {
		t.Fatalf("expected ErrExists when a blocked row already exists, got %v", err)
	}
}

func TestGetDeniedForNonParticipant(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	rel, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_, err = svc.Get(ctx, "u3", rel.ID)
	if !errors.Is(err, relationships.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestListMineFiltersByStatusAndType(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Request(ctx, req("app-1", "u1", "u2", "friend")); err != nil {
		t.Fatalf("request 1: %v", err)
	}
	if _, err := svc.Request(ctx, req("app-1", "u1", "u3", "follow")); err != nil {
		t.Fatalf("request 2: %v", err)
	}
	pendingFriends, err := svc.ListMine(ctx, "app-1", "u1", relationships.ListFilter{Type: "friend", Status: relationships.StatusPending})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pendingFriends) != 1 || pendingFriends[0].Type != "friend" {
		t.Fatalf("expected 1 pending friend relationship, got %+v", pendingFriends)
	}
}
