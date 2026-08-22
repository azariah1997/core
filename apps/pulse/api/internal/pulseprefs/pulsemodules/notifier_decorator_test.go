package pulsemodules_test

import (
	"context"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/memory"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/pulsemodules"
)

type capturedNotify struct {
	called bool
	title  string
	body   string
	data   map[string]any
}

type fakeNotifier struct {
	calls []capturedNotify
}

func (f *fakeNotifier) Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error {
	f.calls = append(f.calls, capturedNotify{called: true, title: title, body: body, data: data})
	return nil
}

func TestDetailedIsThePassthroughDefault(t *testing.T) {
	prefs := pulseprefs.NewService(memory.New())
	inner := &fakeNotifier{}
	decorator := pulsemodules.NewNotifierDecorator(inner, prefs)

	if err := decorator.Notify(context.Background(), "receiver-1", "pulse_received", "Alice pulsed you", "felt for 842ms", map[string]any{"durationMs": 842}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(inner.calls) != 1 || inner.calls[0].title != "Alice pulsed you" || inner.calls[0].body != "felt for 842ms" {
		t.Fatalf("expected the real title/body passed through untouched by default, got %+v", inner.calls)
	}
}

func TestSilentSuppressesTheNotificationEntirely(t *testing.T) {
	prefs := pulseprefs.NewService(memory.New())
	if _, err := prefs.Set(context.Background(), "receiver-1", pulseprefs.SetPreferencesInput{NotificationDetail: pulseprefs.SilentNotification, HapticIntensity: 1.0}); err != nil {
		t.Fatalf("set prefs: %v", err)
	}
	inner := &fakeNotifier{}
	decorator := pulsemodules.NewNotifierDecorator(inner, prefs)

	if err := decorator.Notify(context.Background(), "receiver-1", "pulse_received", "Alice pulsed you", "felt for 842ms", nil); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("expected silent to suppress the real notification entirely, got %+v", inner.calls)
	}
}

func TestPrivateRedactsTheRealContentButStillNotifies(t *testing.T) {
	prefs := pulseprefs.NewService(memory.New())
	if _, err := prefs.Set(context.Background(), "receiver-1", pulseprefs.SetPreferencesInput{NotificationDetail: pulseprefs.PrivateNotification, HapticIntensity: 1.0}); err != nil {
		t.Fatalf("set prefs: %v", err)
	}
	inner := &fakeNotifier{}
	decorator := pulsemodules.NewNotifierDecorator(inner, prefs)

	if err := decorator.Notify(context.Background(), "receiver-1", "pulse_received", "Alice pulsed you", "felt for 842ms", map[string]any{"durationMs": 842}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("expected private to still send a notification, got %+v", inner.calls)
	}
	if inner.calls[0].title == "Alice pulsed you" || inner.calls[0].body == "felt for 842ms" {
		t.Fatalf("expected private to redact the real sender/detail content, got %+v", inner.calls[0])
	}
	if inner.calls[0].data != nil {
		t.Fatalf("expected private to also strip the real structured data payload, got %+v", inner.calls[0].data)
	}
}

func TestANotifierUntouchedUserStillGetsDetailedNotifications(t *testing.T) {
	prefs := pulseprefs.NewService(memory.New())
	inner := &fakeNotifier{}
	decorator := pulsemodules.NewNotifierDecorator(inner, prefs)

	if err := decorator.Notify(context.Background(), "never-configured", "knock_received", "Bob knocked", "", nil); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(inner.calls) != 1 || inner.calls[0].title != "Bob knocked" {
		t.Fatalf("expected a user who never set preferences to still get the real default (detailed), got %+v", inner.calls)
	}
}
