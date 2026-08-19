package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/analytics"
	"github.com/example/core-platform/backend/core-api/internal/analytics/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeLimiter struct{ allow bool }

func (f fakeLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return f.allow, nil
}

func newService(admins map[string]bool, allow bool) *analytics.Service {
	return analytics.NewService(memory.New(), fakeAdmin{admins: admins}, fakeLimiter{allow: allow})
}

func TestTrackRequiresEventName(t *testing.T) {
	svc := newService(nil, true)
	err := svc.Track(context.Background(), "1.2.3.4", []analytics.TrackInput{{AppID: "app1", UserID: "u1"}})
	var verr *analytics.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestTrackRequiresUserOrAnonymousID(t *testing.T) {
	svc := newService(nil, true)
	err := svc.Track(context.Background(), "1.2.3.4", []analytics.TrackInput{{EventName: "screen_viewed", AppID: "app1"}})
	var verr *analytics.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError when neither userId nor anonymousId is set, got %v", err)
	}
}

func TestTrackAcceptsAnonymousEvent(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	ctx := context.Background()
	err := svc.Track(ctx, "1.2.3.4", []analytics.TrackInput{
		{EventName: "app_opened", AppID: "app1", AnonymousID: "anon-xyz"},
	})
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	list, err := svc.ListRecent(ctx, "admin", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 recorded event, got %v err=%v", list, err)
	}
	if list[0].AnonymousID != "anon-xyz" || list[0].UserID != "" {
		t.Fatalf("unexpected event: %+v", list[0])
	}
}

func TestTrackRecordsAWholeBatch(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	ctx := context.Background()
	err := svc.Track(ctx, "1.2.3.4", []analytics.TrackInput{
		{EventName: "screen_viewed", AppID: "app1", UserID: "u1"},
		{EventName: "button_clicked", AppID: "app1", UserID: "u1"},
		{EventName: "purchase_completed", AppID: "app1", UserID: "u1", Properties: map[string]any{"amount": 999}},
	})
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	list, err := svc.ListRecent(ctx, "admin", 10)
	if err != nil || len(list) != 3 {
		t.Fatalf("expected all 3 events in the batch recorded, got %v err=%v", list, err)
	}
}

func TestTrackIsRateLimited(t *testing.T) {
	svc := newService(nil, false)
	err := svc.Track(context.Background(), "1.2.3.4", []analytics.TrackInput{
		{EventName: "screen_viewed", AppID: "app1", UserID: "u1"},
	})
	if !errors.Is(err, analytics.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestTrackDefaultsOccurredAtWhenNotProvided(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	ctx := context.Background()
	before := time.Now().UTC()
	if err := svc.Track(ctx, "1.2.3.4", []analytics.TrackInput{{EventName: "screen_viewed", AppID: "app1", UserID: "u1"}}); err != nil {
		t.Fatalf("track: %v", err)
	}
	list, err := svc.ListRecent(ctx, "admin", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}
	if list[0].OccurredAt.Before(before) {
		t.Fatalf("expected OccurredAt to default to roughly now, got %v (before was %v)", list[0].OccurredAt, before)
	}
}

func TestTrackPreservesClientSuppliedTimestamp(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	ctx := context.Background()
	past := time.Now().UTC().Add(-24 * time.Hour)
	if err := svc.Track(ctx, "1.2.3.4", []analytics.TrackInput{
		{EventName: "screen_viewed", AppID: "app1", UserID: "u1", OccurredAt: past},
	}); err != nil {
		t.Fatalf("track: %v", err)
	}
	list, err := svc.ListRecent(ctx, "admin", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}
	if !list[0].OccurredAt.Equal(past) {
		t.Fatalf("expected the client-supplied OccurredAt to be preserved, got %v want %v", list[0].OccurredAt, past)
	}
}

func TestListRecentRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	ctx := context.Background()
	if err := svc.Track(ctx, "1.2.3.4", []analytics.TrackInput{{EventName: "screen_viewed", AppID: "app1", UserID: "u1"}}); err != nil {
		t.Fatalf("track: %v", err)
	}
	if _, err := svc.ListRecent(ctx, "u1", 10); !errors.Is(err, analytics.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	if _, err := svc.ListRecent(ctx, "admin", 10); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

// There is deliberately no TestListRecentSupportsAggregation or
// anything resembling one - that's the point this package's own doc
// comment and README.md both make explicitly.
