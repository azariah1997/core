package pulseconnections_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/memory"
)

// fakeCoreRelationships is an in-memory stand-in for Core's real
// relationships API - proves pulse-connections' own logic (merging,
// classification defaults, direction) without a real network call.
// The real coresdk-backed adapter (internal/pulseconnections/core) is
// exercised by live curl validation, the same split every other Core
// module's tests vs. live-validation already uses.
type fakeCoreRelationships struct {
	byID map[string]pulseconnections.RelationshipRef
	next int
}

func newFakeCoreRelationships() *fakeCoreRelationships {
	return &fakeCoreRelationships{byID: map[string]pulseconnections.RelationshipRef{}}
}

func (f *fakeCoreRelationships) Request(ctx context.Context, targetUserID, relType string) (pulseconnections.RelationshipRef, error) {
	f.next++
	id := fmt.Sprintf("rel-%d", f.next)
	rel := pulseconnections.RelationshipRef{
		ID: id, RequesterID: "caller-1", TargetID: targetUserID, Status: "pending",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	f.byID[id] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Accept(ctx context.Context, relationshipID string) (pulseconnections.RelationshipRef, error) {
	rel, ok := f.byID[relationshipID]
	if !ok {
		return pulseconnections.RelationshipRef{}, pulseconnections.ErrNotFound
	}
	rel.Status = "active"
	f.byID[relationshipID] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Decline(ctx context.Context, relationshipID string) (pulseconnections.RelationshipRef, error) {
	rel, ok := f.byID[relationshipID]
	if !ok {
		return pulseconnections.RelationshipRef{}, pulseconnections.ErrNotFound
	}
	rel.Status = "ended"
	f.byID[relationshipID] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Remove(ctx context.Context, relationshipID string) error {
	if _, ok := f.byID[relationshipID]; !ok {
		return pulseconnections.ErrNotFound
	}
	delete(f.byID, relationshipID)
	return nil
}

func (f *fakeCoreRelationships) ListMine(ctx context.Context, relType string) ([]pulseconnections.RelationshipRef, error) {
	out := make([]pulseconnections.RelationshipRef, 0, len(f.byID))
	for _, rel := range f.byID {
		out = append(out, rel)
	}
	return out, nil
}

func newService() *pulseconnections.Service {
	return pulseconnections.NewService(memory.New())
}

func TestRequestConnectionRejectsAMissingTarget(t *testing.T) {
	svc := newService()
	_, err := svc.RequestConnection(context.Background(), newFakeCoreRelationships(), "caller-1", pulseconnections.RequestInput{})
	var verr *pulseconnections.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestRequestConnectionDefaultsToFriendClassification(t *testing.T) {
	svc := newService()
	conn, err := svc.RequestConnection(context.Background(), newFakeCoreRelationships(), "caller-1", pulseconnections.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if conn.Classification != pulseconnections.ClassificationFriend {
		t.Fatalf("expected default classification friend, got %v", conn.Classification)
	}
	if conn.Direction != "outgoing" {
		t.Fatalf("expected outgoing direction for the requester, got %v", conn.Direction)
	}
}

func TestAcceptTransitionsStatusToActive(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	created, err := svc.RequestConnection(context.Background(), core, "caller-1", pulseconnections.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	accepted, err := svc.Accept(context.Background(), core, "user-2", created.RelationshipID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Status != "active" {
		t.Fatalf("expected active status, got %v", accepted.Status)
	}
	if accepted.Direction != "incoming" {
		t.Fatalf("expected incoming direction from the target's perspective, got %v", accepted.Direction)
	}
}

func TestSetClassificationRejectsAnInvalidValue(t *testing.T) {
	svc := newService()
	_, err := svc.SetClassification(context.Background(), "caller-1", "rel-1", pulseconnections.SetClassificationInput{Classification: "best_friend_forever"})
	var verr *pulseconnections.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestSetClassificationIsPerOwnerAndReflectedInListMine(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	created, err := svc.RequestConnection(context.Background(), core, "caller-1", pulseconnections.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.SetClassification(context.Background(), "caller-1", created.RelationshipID, pulseconnections.SetClassificationInput{Classification: pulseconnections.ClassificationCloseFriend}); err != nil {
		t.Fatalf("set classification: %v", err)
	}

	mine, err := svc.ListMine(context.Background(), core, "caller-1")
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].Classification != pulseconnections.ClassificationCloseFriend {
		t.Fatalf("expected the close_friend classification to be reflected, got %+v", mine)
	}

	// The classification is caller-1's own view - user-2 never set one,
	// so their own list still defaults to friend for the same relationship.
	theirs, err := svc.ListMine(context.Background(), core, "user-2")
	if err != nil {
		t.Fatalf("list mine (other side): %v", err)
	}
	if len(theirs) != 1 || theirs[0].Classification != pulseconnections.ClassificationFriend {
		t.Fatalf("expected the other side's default classification to remain friend, got %+v", theirs)
	}
}
