package livetouch_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
	"github.com/example/core-platform/apps/pulse/api/internal/livetouch/memory"
)

type fakeBond struct {
	partnerID string
	hasBond   bool
	err       error
}

func (f fakeBond) MyActiveBond(ctx context.Context, callerID string) (livetouch.BondRef, error) {
	if f.err != nil {
		return livetouch.BondRef{}, f.err
	}
	if !f.hasBond {
		return livetouch.BondRef{}, livetouch.ErrNoBond
	}
	return livetouch.BondRef{UserAID: callerID, UserBID: f.partnerID}, nil
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
}

func (f *fakeNotifier) Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, receiverUserID)
	return nil
}

func newService(bond livetouch.Bond, realtime livetouch.Realtime, allow bool) *livetouch.Service {
	return livetouch.NewService(memory.New(), bond, realtime, &fakeAnalytics{}, fakeRateLimiter{allow: allow})
}

func TestInviteRequiresAnActiveBond(t *testing.T) {
	svc := newService(fakeBond{hasBond: false}, &fakeRealtime{}, true)
	_, err := svc.Invite(context.Background(), fakePresence{}, &fakeNotifier{}, "caller-1")
	if !errors.Is(err, livetouch.ErrNotBonded) {
		t.Fatalf("expected ErrNotBonded, got %v", err)
	}
}

func TestInviteResolvesToTheRealBondPartner(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if s.ReceiverID != "partner-1" || s.InitiatorID != "caller-1" {
		t.Fatalf("expected the invite to target the real bond partner, got %+v", s)
	}
	if s.Status != livetouch.StatusInvited {
		t.Fatalf("expected StatusInvited, got %v", s.Status)
	}
}

func TestInviteIsRateLimited(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, false)
	_, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if !errors.Is(err, livetouch.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestInviteDeliversLiveWhenReceiverIsOnline(t *testing.T) {
	realtime := &fakeRealtime{}
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, realtime, true)
	notifier := &fakeNotifier{}
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, notifier, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if s.DeliveryMode != livetouch.DeliveryLive {
		t.Fatalf("expected DeliveryLive, got %v", s.DeliveryMode)
	}
	realtime.mu.Lock()
	defer realtime.mu.Unlock()
	if len(realtime.sent) != 1 {
		t.Fatalf("expected exactly one live realtime push, got %v", realtime.sent)
	}
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.sent) != 0 {
		t.Fatalf("expected no push notification when the invite was delivered live, got %v", notifier.sent)
	}
}

func TestInviteFallsBackToPushWhenReceiverIsOffline(t *testing.T) {
	realtime := &fakeRealtime{}
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, realtime, true)
	notifier := &fakeNotifier{}
	s, err := svc.Invite(context.Background(), fakePresence{online: false}, notifier, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if s.DeliveryMode != livetouch.DeliveryPush {
		t.Fatalf("expected DeliveryPush, got %v", s.DeliveryMode)
	}
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.sent) != 1 || notifier.sent[0] != "partner-1" {
		t.Fatalf("expected exactly one push notification to partner-1, got %v", notifier.sent)
	}
}

func TestOnlyTheReceiverMayAccept(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.Accept(context.Background(), "caller-1", s.ID); !errors.Is(err, livetouch.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for the initiator trying to accept their own invite, got %v", err)
	}
}

func TestAcceptTransitionsToActiveAndExposesAChannel(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	accepted, err := svc.Accept(context.Background(), "partner-1", s.ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Status != livetouch.StatusActive {
		t.Fatalf("expected StatusActive, got %v", accepted.Status)
	}
	if accepted.Channel() != "pulse:live-touch:"+s.ID {
		t.Fatalf("expected a deterministic channel name, got %v", accepted.Channel())
	}
}

func TestOnlyTheReceiverMayDecline(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.Decline(context.Background(), "caller-1", s.ID); !errors.Is(err, livetouch.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	declined, err := svc.Decline(context.Background(), "partner-1", s.ID)
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if declined.Status != livetouch.StatusEnded || declined.EndReason == nil || *declined.EndReason != livetouch.EndReasonDeclined {
		t.Fatalf("expected ended/declined, got status=%v reason=%v", declined.Status, declined.EndReason)
	}
}

func TestEndComputesARealDurationFromAcceptToEnd(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.Accept(context.Background(), "partner-1", s.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	ended, err := svc.End(context.Background(), "caller-1", s.ID)
	if err != nil {
		t.Fatalf("end: %v", err)
	}
	if ended.Status != livetouch.StatusEnded || ended.EndReason == nil || *ended.EndReason != livetouch.EndReasonEnded {
		t.Fatalf("expected ended/ended, got status=%v reason=%v", ended.Status, ended.EndReason)
	}
	if ended.DurationMs == nil || *ended.DurationMs <= 0 {
		t.Fatalf("expected a real positive duration, got %v", ended.DurationMs)
	}
}

func TestEndOnAnUnacceptedInviteHasNoDuration(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	ended, err := svc.End(context.Background(), "caller-1", s.ID)
	if err != nil {
		t.Fatalf("end: %v", err)
	}
	if ended.DurationMs != nil {
		t.Fatalf("expected no duration for a session that was never accepted, got %v", *ended.DurationMs)
	}
}

func TestAStrangerCannotEndSomeoneElsesSession(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.End(context.Background(), "stranger", s.ID); !errors.Is(err, livetouch.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestEndTwiceFails(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.End(context.Background(), "caller-1", s.ID); err != nil {
		t.Fatalf("first end: %v", err)
	}
	if _, err := svc.End(context.Background(), "caller-1", s.ID); !errors.Is(err, livetouch.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition on a second End, got %v", err)
	}
}

func TestGetForbidsANonParticipant(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	s, err := svc.Invite(context.Background(), fakePresence{online: true}, &fakeNotifier{}, "caller-1")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if _, err := svc.Get(context.Background(), "stranger", s.ID); !errors.Is(err, livetouch.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if _, err := svc.Get(context.Background(), "partner-1", s.ID); err != nil {
		t.Fatalf("expected the receiver to be able to Get, got %v", err)
	}
}
