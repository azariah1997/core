package pulseprefs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/memory"
)

type fakeQuietHours struct {
	current pulseprefs.QuietHours
}

func (f *fakeQuietHours) Get(ctx context.Context) (pulseprefs.QuietHours, error) {
	return f.current, nil
}

func (f *fakeQuietHours) Set(ctx context.Context, in pulseprefs.SetQuietHoursInput) (pulseprefs.QuietHours, error) {
	f.current = pulseprefs.QuietHours{Timezone: in.Timezone, StartMinute: in.StartMinute, EndMinute: in.EndMinute, Enabled: in.Enabled}
	return f.current, nil
}

type fakeMutes struct {
	muted map[string]pulseprefs.Mute
}

func newFakeMutes() *fakeMutes {
	return &fakeMutes{muted: map[string]pulseprefs.Mute{}}
}

func (f *fakeMutes) Mute(ctx context.Context, mutedUserID string) (pulseprefs.Mute, error) {
	m := pulseprefs.Mute{ID: "mute-" + mutedUserID, MutedUserID: mutedUserID}
	f.muted[mutedUserID] = m
	return m, nil
}

func (f *fakeMutes) List(ctx context.Context) ([]pulseprefs.Mute, error) {
	var out []pulseprefs.Mute
	for _, m := range f.muted {
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeMutes) Unmute(ctx context.Context, mutedUserID string) error {
	delete(f.muted, mutedUserID)
	return nil
}

func newService() *pulseprefs.Service {
	return pulseprefs.NewService(memory.New())
}

func TestGetReturnsDefaultsWhenNothingHasBeenSet(t *testing.T) {
	svc := newService()
	p, err := svc.Get(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.NotificationDetail != pulseprefs.DetailedNotification || p.HapticIntensity != 1.0 {
		t.Fatalf("expected the real defaults (detailed, full intensity), got %+v", p)
	}
}

func TestSetRejectsAnInvalidNotificationDetail(t *testing.T) {
	svc := newService()
	_, err := svc.Set(context.Background(), "user-a", pulseprefs.SetPreferencesInput{NotificationDetail: "loud", HapticIntensity: 1.0})
	var verr *pulseprefs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a ValidationError, got %v", err)
	}
}

func TestSetRejectsAnOutOfRangeHapticIntensity(t *testing.T) {
	svc := newService()
	_, err := svc.Set(context.Background(), "user-a", pulseprefs.SetPreferencesInput{NotificationDetail: pulseprefs.DetailedNotification, HapticIntensity: 1.5})
	var verr *pulseprefs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a ValidationError, got %v", err)
	}
}

func TestSetPersistsAndGetRoundTrips(t *testing.T) {
	svc := newService()
	saved, err := svc.Set(context.Background(), "user-a", pulseprefs.SetPreferencesInput{NotificationDetail: pulseprefs.PrivateNotification, HapticIntensity: 0.4})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if saved.NotificationDetail != pulseprefs.PrivateNotification || saved.HapticIntensity != 0.4 {
		t.Fatalf("expected the real values echoed back, got %+v", saved)
	}
	got, err := svc.Get(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.NotificationDetail != pulseprefs.PrivateNotification || got.HapticIntensity != 0.4 {
		t.Fatalf("expected the persisted values on re-read, got %+v", got)
	}
	// A different user's preferences are entirely unaffected.
	other, err := svc.Get(context.Background(), "user-b")
	if err != nil {
		t.Fatalf("get other: %v", err)
	}
	if other.NotificationDetail != pulseprefs.DetailedNotification {
		t.Fatalf("expected an untouched user to still see the real defaults, got %+v", other)
	}
}

func TestQuietHoursRoundTripsThroughTheCoreAdapter(t *testing.T) {
	svc := newService()
	core := &fakeQuietHours{}
	saved, err := svc.SetQuietHours(context.Background(), core, pulseprefs.SetQuietHoursInput{Timezone: "America/New_York", StartMinute: 1320, EndMinute: 420, Enabled: true})
	if err != nil {
		t.Fatalf("set quiet hours: %v", err)
	}
	if saved.Timezone != "America/New_York" || !saved.Enabled {
		t.Fatalf("expected the real values echoed back, got %+v", saved)
	}
	got, err := svc.GetQuietHours(context.Background(), core)
	if err != nil {
		t.Fatalf("get quiet hours: %v", err)
	}
	if got != saved {
		t.Fatalf("expected the persisted values on re-read, got %+v vs %+v", got, saved)
	}
}

func TestSetQuietHoursRejectsAnInvalidTimezone(t *testing.T) {
	svc := newService()
	core := &fakeQuietHours{}
	_, err := svc.SetQuietHours(context.Background(), core, pulseprefs.SetQuietHoursInput{Timezone: "", StartMinute: 0, EndMinute: 60, Enabled: true})
	var verr *pulseprefs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a ValidationError, got %v", err)
	}
}

func TestMuteListUnmuteRoundTripThroughTheCoreAdapter(t *testing.T) {
	svc := newService()
	core := newFakeMutes()

	if _, err := svc.Mute(context.Background(), core, "user-b"); err != nil {
		t.Fatalf("mute: %v", err)
	}
	list, err := svc.ListMutes(context.Background(), core)
	if err != nil {
		t.Fatalf("list mutes: %v", err)
	}
	if len(list) != 1 || list[0].MutedUserID != "user-b" {
		t.Fatalf("expected the real muted user in the list, got %+v", list)
	}

	if err := svc.Unmute(context.Background(), core, "user-b"); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	list, err = svc.ListMutes(context.Background(), core)
	if err != nil {
		t.Fatalf("list mutes after unmute: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no mutes left after unmute, got %+v", list)
	}
}
