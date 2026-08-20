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

// fakeCoreGroups is an in-memory stand-in for Core's real groups API -
// the real coresdk-backed adapter (internal/pulseconnections/core) is
// exercised by the http_test.go real-router tests and live curl
// validation, the same split fakeCoreRelationships above uses.
type fakeCoreGroups struct {
	byID    map[string]pulseconnections.Circle
	members map[string][]pulseconnections.CircleMember
	next    int
}

func newFakeCoreGroups() *fakeCoreGroups {
	return &fakeCoreGroups{byID: map[string]pulseconnections.Circle{}, members: map[string][]pulseconnections.CircleMember{}}
}

func (f *fakeCoreGroups) Create(ctx context.Context, name string) (pulseconnections.Circle, error) {
	f.next++
	id := fmt.Sprintf("circle-%d", f.next)
	c := pulseconnections.Circle{ID: id, Name: name, Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.byID[id] = c
	return c, nil
}

func (f *fakeCoreGroups) ListMine(ctx context.Context) ([]pulseconnections.Circle, error) {
	out := make([]pulseconnections.Circle, 0, len(f.byID))
	for _, c := range f.byID {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCoreGroups) ListMembers(ctx context.Context, circleID string) ([]pulseconnections.CircleMember, error) {
	return f.members[circleID], nil
}

func (f *fakeCoreGroups) AddMember(ctx context.Context, circleID, userID string) (pulseconnections.CircleMember, error) {
	m := pulseconnections.CircleMember{UserID: userID, CreatedAt: time.Now().UTC()}
	f.members[circleID] = append(f.members[circleID], m)
	return m, nil
}

func (f *fakeCoreGroups) RemoveMember(ctx context.Context, circleID, userID string) error {
	var kept []pulseconnections.CircleMember
	for _, m := range f.members[circleID] {
		if m.UserID != userID {
			kept = append(kept, m)
		}
	}
	f.members[circleID] = kept
	return nil
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

func TestCreateCircleRejectsAnEmptyName(t *testing.T) {
	svc := newService()
	_, err := svc.CreateCircle(context.Background(), newFakeCoreGroups(), pulseconnections.CreateCircleInput{Name: "  "})
	var verr *pulseconnections.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestAddCircleMemberRequiresAnActiveConnection(t *testing.T) {
	svc := newService()
	groups := newFakeCoreGroups()
	circle, err := svc.CreateCircle(context.Background(), groups, pulseconnections.CreateCircleInput{Name: "Family"})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	// No relationship exists between caller-1 and user-2 at all.
	_, err = svc.AddCircleMember(context.Background(), groups, newFakeCoreRelationships(), circle.ID, "user-2")
	if !errors.Is(err, pulseconnections.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestAddCircleMemberSucceedsForARealActiveConnection(t *testing.T) {
	svc := newService()
	groups := newFakeCoreGroups()
	core := newFakeCoreRelationships()

	circle, err := svc.CreateCircle(context.Background(), groups, pulseconnections.CreateCircleInput{Name: "Family"})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	created, err := svc.RequestConnection(context.Background(), core, "caller-1", pulseconnections.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request connection: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "user-2", created.RelationshipID); err != nil {
		t.Fatalf("accept: %v", err)
	}

	member, err := svc.AddCircleMember(context.Background(), groups, core, circle.ID, "user-2")
	if err != nil {
		t.Fatalf("expected a real active connection to be addable to a Circle, got %v", err)
	}
	if member.UserID != "user-2" {
		t.Fatalf("expected the real added member, got %+v", member)
	}

	members, err := svc.ListCircleMembers(context.Background(), groups, circle.ID)
	if err != nil {
		t.Fatalf("list circle members: %v", err)
	}
	if len(members) != 1 || members[0].UserID != "user-2" {
		t.Fatalf("expected user-2 in the Circle's member list, got %+v", members)
	}

	if err := svc.RemoveCircleMember(context.Background(), groups, circle.ID, "user-2"); err != nil {
		t.Fatalf("remove circle member: %v", err)
	}
	members, err = svc.ListCircleMembers(context.Background(), groups, circle.ID)
	if err != nil {
		t.Fatalf("list circle members after remove: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected the Circle to have no members after remove, got %+v", members)
	}
}
