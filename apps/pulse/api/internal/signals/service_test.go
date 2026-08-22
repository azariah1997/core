package signals_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/signals"
	"github.com/example/core-platform/apps/pulse/api/internal/signals/memory"
)

type fakeCoreRelationships struct {
	byType map[string][]signals.RelationshipRef
}

func newFakeCoreRelationships() *fakeCoreRelationships {
	return &fakeCoreRelationships{byType: map[string][]signals.RelationshipRef{}}
}

func (f *fakeCoreRelationships) connect(relType, otherUserID, status string) {
	f.byType[relType] = append(f.byType[relType], signals.RelationshipRef{RequesterID: "caller-1", TargetID: otherUserID, Status: status})
}

func (f *fakeCoreRelationships) ListMine(ctx context.Context, relType string) ([]signals.RelationshipRef, error) {
	return f.byType[relType], nil
}

func newService() *signals.Service {
	return signals.NewService(memory.New())
}

func validSegments() []signals.Segment {
	return []signals.Segment{
		{Type: signals.SegmentTap, DurationMs: 150},
		{Type: signals.SegmentPause, DurationMs: 300},
		{Type: signals.SegmentHold, DurationMs: 900},
	}
}

func TestCreateRequiresAnExistingConnection(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships() // no connection
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: validSegments()})
	if !errors.Is(err, signals.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestCreateRejectsEmptySegments(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2"})
	var verr *signals.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for empty segments, got %v", err)
	}
}

func TestCreateRejectsTooManySegments(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	var segs []signals.Segment
	for i := 0; i < 21; i++ {
		segs = append(segs, signals.Segment{Type: signals.SegmentTap, DurationMs: 50})
	}
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: segs})
	var verr *signals.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for too many segments, got %v", err)
	}
}

func TestCreateRejectsASegmentDurationOutOfBounds(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{
		TargetUserID: "user-2", Segments: []signals.Segment{{Type: signals.SegmentTap, DurationMs: 5000}},
	})
	var verr *signals.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for an out-of-bounds segment duration, got %v", err)
	}
}

func TestCreateRejectsExcessiveTotalDuration(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	var segs []signals.Segment
	for i := 0; i < 15; i++ {
		segs = append(segs, signals.Segment{Type: signals.SegmentHold, DurationMs: 900}) // 13500ms total
	}
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: segs})
	var verr *signals.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for excessive total duration, got %v", err)
	}
}

func TestCreateRejectsAnInvalidSegmentType(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	_, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{
		TargetUserID: "user-2", Segments: []signals.Segment{{Type: "wiggle", DurationMs: 100}},
	})
	var verr *signals.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for an invalid segment type, got %v", err)
	}
}

func TestCreateSucceedsForARealConnectionWithinBounds(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	sig, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Label: "I love you", Segments: validSegments()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sig.OwnerUserID != "caller-1" || sig.TargetUserID != "user-2" || sig.Label != "I love you" {
		t.Fatalf("expected the real owner/target/label to be recorded, got %+v", sig)
	}
	if len(sig.Segments) != 3 {
		t.Fatalf("expected all 3 segments to be persisted, got %d", len(sig.Segments))
	}
}

func TestOnlyTheOwnerMayGetTheirSignal(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	sig, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: validSegments()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), "user-2", sig.ID); !errors.Is(err, signals.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-owner (even the pattern's own target), got %v", err)
	}
	got, err := svc.Get(context.Background(), "caller-1", sig.ID)
	if err != nil {
		t.Fatalf("expected the real owner to Get their own signal, got %v", err)
	}
	if got.ID != sig.ID {
		t.Fatalf("expected the same signal back, got %+v", got)
	}
}

func TestOnlyTheOwnerMayDelete(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	sig, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: validSegments()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(context.Background(), "user-2", sig.ID); !errors.Is(err, signals.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(context.Background(), "caller-1", sig.ID); err != nil {
		t.Fatalf("expected the real owner to delete their own signal, got %v", err)
	}
	if _, err := svc.Get(context.Background(), "caller-1", sig.ID); !errors.Is(err, signals.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListMineOnlyReturnsTheCallersOwnSignals(t *testing.T) {
	svc := newService()
	core := newFakeCoreRelationships()
	core.connect(signals.FriendRelationshipType, "user-2", "active")
	if _, err := svc.Create(context.Background(), core, "caller-1", signals.CreateInput{TargetUserID: "user-2", Segments: validSegments()}); err != nil {
		t.Fatalf("create: %v", err)
	}
	mine, err := svc.ListMine(context.Background(), "caller-1")
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(mine))
	}
	theirs, err := svc.ListMine(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("list mine (target's own): %v", err)
	}
	if len(theirs) != 0 {
		t.Fatalf("expected the target to have no signals of their own (they didn't create any), got %d", len(theirs))
	}
}
