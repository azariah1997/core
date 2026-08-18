package notifications_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
	"github.com/example/core-platform/backend/core-api/internal/notifications/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type recordingSender struct {
	channel notifications.Channel
	calls   []notifications.Notification
	fail    error
}

func (s *recordingSender) Send(ctx context.Context, n notifications.Notification) (string, error) {
	s.calls = append(s.calls, n)
	if s.fail != nil {
		return "", s.fail
	}
	return "ref:" + n.ID, nil
}

func newService(senders map[notifications.Channel]notifications.ChannelSender, admins map[string]bool) *notifications.Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return notifications.NewService(memory.New(), senders, fakeAdmin{admins: admins}, logger)
}

func TestSendRejectsNotifyingSomeoneElseWithoutAdmin(t *testing.T) {
	svc := newService(nil, nil)
	_, _, err := svc.Send(context.Background(), "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u2", Category: "message", Channels: []notifications.Channel{notifications.ChannelInApp},
		Title: "Hi", Body: "there",
	})
	if !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSendAllowsSelfNotify(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, nil)
	n, deliveries, err := svc.Send(context.Background(), "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message", Channels: []notifications.Channel{notifications.ChannelInApp},
		Title: "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if n.UserID != "u1" || len(deliveries) != 1 {
		t.Fatalf("unexpected result: %+v %+v", n, deliveries)
	}
	if deliveries[0].Status != notifications.StatusSent {
		t.Fatalf("expected sent status, got %v", deliveries[0].Status)
	}
}

func TestSendAllowsAdminToNotifyAnyone(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, map[string]bool{"admin": true})
	_, _, err := svc.Send(context.Background(), "admin", notifications.SendInput{
		AppID: "app-1", UserID: "u2", Category: "message", Channels: []notifications.Channel{notifications.ChannelInApp},
		Title: "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("expected admin to be able to notify anyone, got %v", err)
	}
}

func TestSendDispatchesToEveryRequestedChannel(t *testing.T) {
	inApp := &recordingSender{}
	realtime := &recordingSender{}
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp:    inApp,
		notifications.ChannelRealtime: realtime,
	}, nil)
	_, deliveries, err := svc.Send(context.Background(), "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message",
		Channels: []notifications.Channel{notifications.ChannelInApp, notifications.ChannelRealtime},
		Title:    "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(inApp.calls) != 1 || len(realtime.calls) != 1 {
		t.Fatalf("expected exactly one call to each sender, got in_app=%d realtime=%d", len(inApp.calls), len(realtime.calls))
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected 2 delivery records, got %d", len(deliveries))
	}
}

func TestSendRecordsFailedDeliveryWithoutFailingTheWholeSend(t *testing.T) {
	failing := &recordingSender{fail: errors.New("boom")}
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelPush: failing,
	}, nil)
	_, deliveries, err := svc.Send(context.Background(), "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message", Channels: []notifications.Channel{notifications.ChannelPush},
		Title: "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("expected Send itself to succeed even though the channel failed, got %v", err)
	}
	if deliveries[0].Status != notifications.StatusFailed || deliveries[0].Error == "" {
		t.Fatalf("expected a failed delivery with an error message, got %+v", deliveries[0])
	}
}

func TestSendSkipsChannelDisabledByPreference(t *testing.T) {
	sender := &recordingSender{}
	svc := newService(map[notifications.Channel]notifications.ChannelSender{notifications.ChannelEmail: sender}, nil)
	ctx := context.Background()

	if _, err := svc.SetPreference(ctx, "u1", "app-1", notifications.SetPreferenceInput{
		Category: "message", Channel: notifications.ChannelEmail, Enabled: false,
	}); err != nil {
		t.Fatalf("set preference: %v", err)
	}

	_, deliveries, err := svc.Send(ctx, "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message", Channels: []notifications.Channel{notifications.ChannelEmail},
		Title: "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if deliveries[0].Status != notifications.StatusSkipped {
		t.Fatalf("expected skipped status, got %v", deliveries[0].Status)
	}
	if len(sender.calls) != 0 {
		t.Fatalf("expected the sender to never be invoked for a disabled channel, got %d calls", len(sender.calls))
	}
}

func TestSendDefersInterruptiveChannelDuringQuietHoursButNotPassiveChannels(t *testing.T) {
	push := &recordingSender{}
	inApp := &recordingSender{}
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelPush:  push,
		notifications.ChannelInApp: inApp,
	}, nil)
	ctx := context.Background()

	// A quiet-hours window covering the entire day, so "now" is always inside it.
	if _, err := svc.SetQuietHours(ctx, "u1", "app-1", notifications.SetQuietHoursInput{
		Timezone: "UTC", StartMinute: 0, EndMinute: 1439, Enabled: true,
	}); err != nil {
		t.Fatalf("set quiet hours: %v", err)
	}

	_, deliveries, err := svc.Send(ctx, "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message",
		Channels: []notifications.Channel{notifications.ChannelPush, notifications.ChannelInApp},
		Title:    "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	byChannel := map[notifications.Channel]notifications.NotificationDelivery{}
	for _, d := range deliveries {
		byChannel[d.Channel] = d
	}
	if byChannel[notifications.ChannelPush].Status != notifications.StatusDeferred {
		t.Fatalf("expected push (interruptive) to be deferred during quiet hours, got %v", byChannel[notifications.ChannelPush].Status)
	}
	if len(push.calls) != 0 {
		t.Fatal("expected push sender to never be invoked while deferred")
	}
	if byChannel[notifications.ChannelInApp].Status != notifications.StatusSent {
		t.Fatalf("expected in_app (passive) to be delivered normally during quiet hours, got %v", byChannel[notifications.ChannelInApp].Status)
	}
}

func TestSendWithTemplateRendersVariables(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, map[string]bool{"admin": true})
	ctx := context.Background()

	if _, err := svc.CreateTemplate(ctx, "admin", notifications.CreateTemplateInput{
		AppID: "app-1", Key: "message.new", TitleTemplate: "{{.actorName}} sent you a message", BodyTemplate: "{{.preview}}",
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}

	n, _, err := svc.Send(ctx, "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message", Channels: []notifications.Channel{notifications.ChannelInApp},
		TemplateKey: "message.new", TemplateData: map[string]any{"actorName": "Alice", "preview": "hey there"},
	})
	if err != nil {
		t.Fatalf("send with template: %v", err)
	}
	if n.Title != "Alice sent you a message" || n.Body != "hey there" {
		t.Fatalf("unexpected rendered notification: %+v", n)
	}
}

func TestTemplateManagementRequiresAdmin(t *testing.T) {
	svc := newService(nil, nil)
	_, err := svc.CreateTemplate(context.Background(), "u1", notifications.CreateTemplateInput{
		AppID: "app-1", Key: "x", TitleTemplate: "a", BodyTemplate: "b",
	})
	if !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin creating a template, got %v", err)
	}
}

func TestRetryDeliveryReDispatches(t *testing.T) {
	sender := &recordingSender{fail: errors.New("temporarily down")}
	svc := newService(map[notifications.Channel]notifications.ChannelSender{notifications.ChannelPush: sender}, nil)
	ctx := context.Background()

	n, deliveries, err := svc.Send(ctx, "u1", notifications.SendInput{
		AppID: "app-1", UserID: "u1", Category: "message", Channels: []notifications.Channel{notifications.ChannelPush},
		Title: "Hi", Body: "there",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if deliveries[0].Status != notifications.StatusFailed {
		t.Fatalf("expected initial send to fail, got %v", deliveries[0].Status)
	}

	sender.fail = nil // the underlying problem is "fixed"
	retried, err := svc.RetryDelivery(ctx, "u1", n.ID, deliveries[0].ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Status != notifications.StatusSent {
		t.Fatalf("expected retry to succeed, got %v", retried.Status)
	}
	if retried.Attempts != 2 {
		t.Fatalf("expected attempts to be incremented to 2, got %d", retried.Attempts)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("expected the sender to have been invoked twice (initial + retry), got %d", len(sender.calls))
	}
}
