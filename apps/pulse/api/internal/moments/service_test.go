package moments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/moments"
	"github.com/example/core-platform/apps/pulse/api/internal/moments/memory"
)

type fakeInteractions struct {
	byID map[string]moments.InteractionRef
}

func newFakeInteractions() *fakeInteractions {
	return &fakeInteractions{byID: map[string]moments.InteractionRef{}}
}

func (f *fakeInteractions) add(ref moments.InteractionRef) {
	f.byID[ref.ID] = ref
}

func (f *fakeInteractions) Get(ctx context.Context, callerID, interactionID string) (moments.InteractionRef, error) {
	ref, ok := f.byID[interactionID]
	if !ok {
		return moments.InteractionRef{}, moments.ErrNotFound
	}
	if ref.SenderID != callerID && ref.ReceiverID != callerID {
		return moments.InteractionRef{}, moments.ErrForbidden
	}
	return ref, nil
}

func newService() *moments.Service {
	return moments.NewService(memory.New())
}

func TestSaveRequiresARealParticipant(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed"})

	_, err := svc.Save(context.Background(), interactions, "stranger", "i-1")
	if !errors.Is(err, moments.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSaveRequiresACompletedInteraction(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "started"})

	_, err := svc.Save(context.Background(), interactions, "user-a", "i-1")
	if !errors.Is(err, moments.ErrNotCompleted) {
		t.Fatalf("expected ErrNotCompleted, got %v", err)
	}
}

func TestSaveRecordsTheRealParticipantsTypeAndDuration(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	durationMs := 842
	interactions.add(moments.InteractionRef{
		ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed",
		Type: "pulse", DurationMs: &durationMs, CreatedAt: time.Now().UTC(),
	})

	m, err := svc.Save(context.Background(), interactions, "user-a", "i-1")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if m.SavedByUserID != "user-a" || m.OtherUserID != "user-b" || m.InteractionType != "pulse" {
		t.Fatalf("expected the real participants/type recorded, got %+v", m)
	}
	if m.DurationMs == nil || *m.DurationMs != 842 {
		t.Fatalf("expected the real duration recorded, got %v", m.DurationMs)
	}
}

func TestSaveResolvesTheOtherUserFromTheReceiversPerspectiveToo(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed", Type: "knock", CreatedAt: time.Now().UTC()})

	m, err := svc.Save(context.Background(), interactions, "user-b", "i-1")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if m.SavedByUserID != "user-b" || m.OtherUserID != "user-a" {
		t.Fatalf("expected the receiver's own save to record the sender as the other participant, got %+v", m)
	}
}

func TestSaveIsIdempotentPerCaller(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed", Type: "pulse", CreatedAt: time.Now().UTC()})

	first, err := svc.Save(context.Background(), interactions, "user-a", "i-1")
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	second, err := svc.Save(context.Background(), interactions, "user-a", "i-1")
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same moment ID on a repeat save, got %v vs %v", first.ID, second.ID)
	}

	mine, err := svc.ListMine(context.Background(), "user-a", 0)
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 {
		t.Fatalf("expected exactly one moment despite saving twice, got %d", len(mine))
	}
}

func TestBothParticipantsCanIndependentlySaveTheSameInteraction(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed", Type: "pulse", CreatedAt: time.Now().UTC()})

	if _, err := svc.Save(context.Background(), interactions, "user-a", "i-1"); err != nil {
		t.Fatalf("user-a save: %v", err)
	}
	if _, err := svc.Save(context.Background(), interactions, "user-b", "i-1"); err != nil {
		t.Fatalf("user-b save: %v", err)
	}

	aMine, _ := svc.ListMine(context.Background(), "user-a", 0)
	bMine, _ := svc.ListMine(context.Background(), "user-b", 0)
	if len(aMine) != 1 || len(bMine) != 1 {
		t.Fatalf("expected each participant to have their own independent saved moment, got a=%d b=%d", len(aMine), len(bMine))
	}
}

func TestOnlyTheSaverMayDelete(t *testing.T) {
	svc := newService()
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed", Type: "pulse", CreatedAt: time.Now().UTC()})

	m, err := svc.Save(context.Background(), interactions, "user-a", "i-1")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := svc.Delete(context.Background(), "user-b", m.ID); !errors.Is(err, moments.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for the other participant trying to delete user-a's own saved moment, got %v", err)
	}
	if err := svc.Delete(context.Background(), "user-a", m.ID); err != nil {
		t.Fatalf("expected the real saver to delete their own moment, got %v", err)
	}
	mine, _ := svc.ListMine(context.Background(), "user-a", 0)
	if len(mine) != 0 {
		t.Fatalf("expected no moments left after delete, got %d", len(mine))
	}
}
