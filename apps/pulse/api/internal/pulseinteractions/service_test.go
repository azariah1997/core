package pulseinteractions_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/memory"
)

type fakeCoreRelationships struct {
	byType map[string][]pulseinteractions.RelationshipRef
}

func newFakeCoreRelationships() *fakeCoreRelationships {
	return &fakeCoreRelationships{byType: map[string][]pulseinteractions.RelationshipRef{}}
}

func (f *fakeCoreRelationships) connect(relType, otherUserID, status string) {
	f.byType[relType] = append(f.byType[relType], pulseinteractions.RelationshipRef{RequesterID: "caller-1", TargetID: otherUserID, Status: status})
}

func (f *fakeCoreRelationships) ListMine(ctx context.Context, relType string) ([]pulseinteractions.RelationshipRef, error) {
	return f.byType[relType], nil
}

type fakeRealtime struct {
	mu   sync.Mutex
	sent []string // "userID:payload"
}

func (f *fakeRealtime) ToUser(ctx context.Context, userID string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, userID+":"+string(payload))
	return nil
}

type fakeAnalytics struct {
	mu     sync.Mutex
	tracks []string
}

func (f *fakeAnalytics) Track(ctx context.Context, eventName, userID string, properties map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tracks = append(f.tracks, eventName+":"+userID)
	return nil
}

type fakeRateLimiter struct {
	allow bool
}

func (f fakeRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return f.allow, nil
}

type fakePresence struct {
	online bool
	err    error
}

func (f fakePresence) IsOnline(ctx context.Context, userID string) (bool, error) {
	return f.online, f.err
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
	fail error
}

func (f *fakeNotifier) NotifyPulseReceived(ctx context.Context, receiverUserID string, durationMs int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, receiverUserID)
	return f.fail
}

func newService(realtime *fakeRealtime, analytics *fakeAnalytics, allow bool) *pulseinteractions.Service {
	return pulseinteractions.NewService(memory.New(), realtime, analytics, fakeRateLimiter{allow: allow})
}

func TestCreateRequiresAnExistingConnection(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships() // no connection
	_, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if !errors.Is(err, pulseinteractions.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestCreateRejectsABlockedRelationship(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "blocked")
	_, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if !errors.Is(err, pulseinteractions.ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestCreateSucceedsOverAnActiveFriendOrBondConnection(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.BondRelationshipType, "user-2", "active")
	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if i.Status != pulseinteractions.StatusCreated {
		t.Fatalf("expected created status, got %v", i.Status)
	}
}

func TestCreateIsRateLimited(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, false)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	_, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if !errors.Is(err, pulseinteractions.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestCreateIsIdempotentOnClientRequestID(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	first, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2", ClientRequestID: "retry-1"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2", ClientRequestID: "retry-1"})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same interaction ID on retry, got %s and %s", first.ID, second.ID)
	}
}

func TestOnlyTheSenderMayStartOrStop(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Start(context.Background(), fakePresence{online: true}, "user-2", i.ID); !errors.Is(err, pulseinteractions.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for the receiver starting, got %v", err)
	}
}

func TestStartCannotBeCalledTwice(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Start(context.Background(), fakePresence{online: true}, "caller-1", i.ID); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := svc.Start(context.Background(), fakePresence{online: true}, "caller-1", i.ID); !errors.Is(err, pulseinteractions.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition on a second start, got %v", err)
	}
}

func TestStopRequiresAPriorStart(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Stop(context.Background(), &fakeNotifier{}, "caller-1", i.ID); !errors.Is(err, pulseinteractions.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition stopping before starting, got %v", err)
	}
}

func TestFullLiveRoundTripComputesServerDurationAndNotifies(t *testing.T) {
	realtime := &fakeRealtime{}
	analytics := &fakeAnalytics{}
	svc := newService(realtime, analytics, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")

	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, err := svc.Start(context.Background(), fakePresence{online: true}, "caller-1", i.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != pulseinteractions.StatusStarted || started.StartedAt == nil {
		t.Fatalf("expected started status with a StartedAt, got %+v", started)
	}
	if started.DeliveryMode != pulseinteractions.DeliveryLive {
		t.Fatalf("expected DeliveryLive when the receiver is online, got %v", started.DeliveryMode)
	}

	time.Sleep(5 * time.Millisecond)
	completed, err := svc.Stop(context.Background(), &fakeNotifier{}, "caller-1", i.ID)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if completed.Status != pulseinteractions.StatusCompleted {
		t.Fatalf("expected completed status, got %v", completed.Status)
	}
	if completed.DurationMs == nil || *completed.DurationMs <= 0 {
		t.Fatalf("expected a real positive server-computed duration, got %v", completed.DurationMs)
	}

	realtime.mu.Lock()
	sentCount := len(realtime.sent)
	realtime.mu.Unlock()
	if sentCount != 2 {
		t.Fatalf("expected 2 realtime pushes (started + stopped), got %d: %v", sentCount, realtime.sent)
	}

	analytics.mu.Lock()
	defer analytics.mu.Unlock()
	if len(analytics.tracks) != 1 || analytics.tracks[0] != "pulse_completed:caller-1" {
		t.Fatalf("expected exactly one pulse_completed analytics event, got %v", analytics.tracks)
	}
}

func TestOfflineReceiverGetsPushDeliveryModeAndNoLiveAttemptAtStart(t *testing.T) {
	realtime := &fakeRealtime{}
	svc := newService(realtime, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")

	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, err := svc.Start(context.Background(), fakePresence{online: false}, "caller-1", i.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.DeliveryMode != pulseinteractions.DeliveryPush {
		t.Fatalf("expected DeliveryPush when the receiver is offline, got %v", started.DeliveryMode)
	}

	realtime.mu.Lock()
	sentCount := len(realtime.sent)
	realtime.mu.Unlock()
	if sentCount != 0 {
		t.Fatalf("expected no live realtime push attempt for an offline receiver, got %d: %v", sentCount, realtime.sent)
	}
}

func TestOfflineReceiverGetsARealPushNotificationOnlyAtStop(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")

	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Start(context.Background(), fakePresence{online: false}, "caller-1", i.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	notifier := &fakeNotifier{}
	if _, err := svc.Stop(context.Background(), notifier, "caller-1", i.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.sent) != 1 || notifier.sent[0] != "user-2" {
		t.Fatalf("expected exactly one push notification to user-2, got %v", notifier.sent)
	}
}

func TestPresenceCheckFailureFailsClosedToPush(t *testing.T) {
	realtime := &fakeRealtime{}
	svc := newService(realtime, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")

	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, err := svc.Start(context.Background(), fakePresence{err: errors.New("realtime-gateway unreachable")}, "caller-1", i.ID)
	if err != nil {
		t.Fatalf("expected Start to still succeed despite a presence-check failure, got %v", err)
	}
	if started.DeliveryMode != pulseinteractions.DeliveryPush {
		t.Fatalf("expected a presence-check failure to fail closed to DeliveryPush, got %v", started.DeliveryMode)
	}
}

func TestAFailedPushNotificationDoesNotFailStop(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")

	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Start(context.Background(), fakePresence{online: false}, "caller-1", i.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	notifier := &fakeNotifier{fail: errors.New("no registered device")}
	completed, err := svc.Stop(context.Background(), notifier, "caller-1", i.ID)
	if err != nil {
		t.Fatalf("expected Stop to succeed even though the push notification failed, got %v", err)
	}
	if completed.Status != pulseinteractions.StatusCompleted {
		t.Fatalf("expected completed status, got %v", completed.Status)
	}
}

func TestGetForbidsAStrangerFromReadingAnInteraction(t *testing.T) {
	svc := newService(&fakeRealtime{}, &fakeAnalytics{}, true)
	core := newFakeCoreRelationships()
	core.connect(pulseinteractions.FriendRelationshipType, "user-2", "active")
	i, err := svc.Create(context.Background(), core, "caller-1", pulseinteractions.CreateInput{ReceiverID: "user-2"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), "stranger", i.ID); !errors.Is(err, pulseinteractions.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger, got %v", err)
	}
	if _, err := svc.Get(context.Background(), "user-2", i.ID); err != nil {
		t.Fatalf("expected the receiver to read it, got %v", err)
	}
}
