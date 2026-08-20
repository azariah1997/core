package bond_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	"github.com/example/core-platform/apps/pulse/api/internal/bond/memory"
)

// fakeCoreRelationships is an in-memory stand-in for Core's real
// relationships API. friendsOf lets a test pre-seed an existing active
// pulse_friend connection, since RequestBond requires one (product spec
// §11) before ever calling Core to create the bond relationship itself.
type fakeCoreRelationships struct {
	byID      map[string]bond.RelationshipRef
	next      int
	friendsOf map[string][]bond.RelationshipRef // requesterID -> their active pulse_friend list
}

func newFakeCoreRelationships() *fakeCoreRelationships {
	return &fakeCoreRelationships{byID: map[string]bond.RelationshipRef{}, friendsOf: map[string][]bond.RelationshipRef{}}
}

func (f *fakeCoreRelationships) seedFriendship(userA, userB string) {
	ref := bond.RelationshipRef{ID: "friend-" + userA + userB, RequesterID: userA, TargetID: userB, Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.friendsOf[userA] = append(f.friendsOf[userA], ref)
}

func (f *fakeCoreRelationships) Request(ctx context.Context, targetUserID, relType string) (bond.RelationshipRef, error) {
	f.next++
	id := fmt.Sprintf("rel-%d", f.next)
	rel := bond.RelationshipRef{ID: id, RequesterID: "caller-1", TargetID: targetUserID, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.byID[id] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Accept(ctx context.Context, relationshipID string) (bond.RelationshipRef, error) {
	rel, ok := f.byID[relationshipID]
	if !ok {
		return bond.RelationshipRef{}, bond.ErrNotFound
	}
	rel.Status = "active"
	f.byID[relationshipID] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Decline(ctx context.Context, relationshipID string) (bond.RelationshipRef, error) {
	rel, ok := f.byID[relationshipID]
	if !ok {
		return bond.RelationshipRef{}, bond.ErrNotFound
	}
	rel.Status = "ended"
	f.byID[relationshipID] = rel
	return rel, nil
}

func (f *fakeCoreRelationships) Remove(ctx context.Context, relationshipID string) error {
	if _, ok := f.byID[relationshipID]; !ok {
		return bond.ErrNotFound
	}
	delete(f.byID, relationshipID)
	return nil
}

func (f *fakeCoreRelationships) ListMine(ctx context.Context, relType string) ([]bond.RelationshipRef, error) {
	if relType == bond.FriendRelationshipType {
		return f.friendsOf["caller-1"], nil
	}
	out := make([]bond.RelationshipRef, 0, len(f.byID))
	for _, rel := range f.byID {
		out = append(out, rel)
	}
	return out, nil
}

func newService() *bond.Service {
	return bond.NewService(memory.New())
}

func TestRequestBondRequiresAnExistingConnection(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships() // no friendship seeded
	_, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-2"})
	if !errors.Is(err, bond.ErrNoConnection) {
		t.Fatalf("expected ErrNoConnection, got %v", err)
	}
}

func TestRequestBondSucceedsOverAnExistingConnection(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.seedFriendship("caller-1", "user-2")
	b, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request bond: %v", err)
	}
	if b.Status != bond.StatusPending {
		t.Fatalf("expected pending status, got %v", b.Status)
	}
}

func TestOnlyTheTargetMayAcceptABond(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.seedFriendship("caller-1", "user-2")
	b, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "caller-1", b.ID); !errors.Is(err, bond.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for the requester accepting their own bond, got %v", err)
	}
	accepted, err := svc.Accept(context.Background(), core, "user-2", b.ID)
	if err != nil {
		t.Fatalf("expected the real target to accept, got %v", err)
	}
	if accepted.Status != bond.StatusActive {
		t.Fatalf("expected active status, got %v", accepted.Status)
	}
}

func TestASecondActiveBondIsRejectedForEitherParticipant(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.seedFriendship("caller-1", "user-2")
	core.seedFriendship("caller-1", "user-3")

	first, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request 1: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "user-2", first.ID); err != nil {
		t.Fatalf("accept 1: %v", err)
	}

	second, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-3"})
	if err != nil {
		t.Fatalf("request 2: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "user-3", second.ID); !errors.Is(err, bond.ErrAlreadyBonded) {
		t.Fatalf("expected ErrAlreadyBonded since caller-1 already holds an active bond, got %v", err)
	}
}

func TestMyActiveBondReturnsNotFoundWhenUnbonded(t *testing.T) {
	svc := newService()
	_, err := svc.MyActiveBond(context.Background(), "caller-1")
	if !errors.Is(err, bond.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEndRemovesTheActiveHolderSlot(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.seedFriendship("caller-1", "user-2")
	b, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-2"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "user-2", b.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := svc.End(context.Background(), core, "caller-1", b.ID); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := svc.MyActiveBond(context.Background(), "caller-1"); !errors.Is(err, bond.ErrNotFound) {
		t.Fatalf("expected no active bond after ending, got %v", err)
	}

	// The freed slot should allow a brand new bond.
	core.seedFriendship("caller-1", "user-4")
	next, err := svc.RequestBond(context.Background(), core, "caller-1", bond.RequestInput{TargetUserID: "user-4"})
	if err != nil {
		t.Fatalf("request after end: %v", err)
	}
	if _, err := svc.Accept(context.Background(), core, "user-4", next.ID); err != nil {
		t.Fatalf("expected accepting a new bond after ending the old one to succeed, got %v", err)
	}
}
