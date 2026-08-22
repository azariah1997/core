// Package pulseprefs is Pulse's Experience Controls (product spec §33,
// §34, §72, Phase 13): "notification detail level" (Detailed/Private/
// Silent) and haptic intensity are Pulse-owned preferences (Core has no
// concept of either), stored here. Quiet Hours and Mute are genuinely
// shared Core capabilities this module never duplicates - Quiet Hours
// *values* live in Core's notifications.QuietHours (scoped by Pulse's
// AppID, already enforced automatically by Core's Send before any push
// leaves the platform) and Mute is Core's platform-wide
// trustsafety.Mute; this module is a thin, Pulse-scoped settings
// surface over both, matching the Architecture Audit's own framing.
package pulseprefs

import (
	"context"
	"errors"
	"strings"
	"time"
)

// NotificationDetail controls how much real content a Pulse-sent push
// notification carries (spec §72) - never a client-side-only setting,
// since it's enforced by the sender-side Notifier before Core's Send is
// ever called (see pulsemodules.NotifierDecorator).
type NotificationDetail string

const (
	// DetailedNotification is today's existing behavior: the real
	// sender/pattern/duration content Notify was already called with.
	DetailedNotification NotificationDetail = "detailed"
	// PrivateNotification replaces the real title/body with a generic,
	// content-free one before Core's Send is called - a notification
	// still arrives (so the receiver isn't confused by total silence),
	// but a phone lock screen never reveals who or what.
	PrivateNotification NotificationDetail = "private"
	// SilentNotification skips calling Notify entirely - the
	// interaction itself still succeeds and is recorded exactly as
	// normal, only the push notification is suppressed.
	SilentNotification NotificationDetail = "silent"
)

func (d NotificationDetail) valid() bool {
	switch d {
	case DetailedNotification, PrivateNotification, SilentNotification:
		return true
	default:
		return false
	}
}

const (
	minHapticIntensity = 0.0
	maxHapticIntensity = 1.0
)

// Preferences is one row per user - Pulse's own experience-control
// settings, never Core's (Core has no concept of either field).
type Preferences struct {
	UserID             string
	NotificationDetail NotificationDetail
	HapticIntensity    float64
	UpdatedAt          time.Time
}

// DefaultPreferences is what every user has before ever setting
// anything - full detail, full intensity, matching today's existing
// (pre-Phase-13) behavior exactly, so shipping this phase changes
// nothing for a user who never opens the new settings screen.
func DefaultPreferences(userID string) Preferences {
	return Preferences{UserID: userID, NotificationDetail: DetailedNotification, HapticIntensity: 1.0}
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var ErrNotFound = errors.New("preferences not found")

type SetPreferencesInput struct {
	NotificationDetail NotificationDetail
	HapticIntensity    float64
}

func (in SetPreferencesInput) Validate() error {
	if !in.NotificationDetail.valid() {
		return &ValidationError{Message: "notificationDetail must be one of: detailed, private, silent"}
	}
	if in.HapticIntensity < minHapticIntensity || in.HapticIntensity > maxHapticIntensity {
		return &ValidationError{Message: "hapticIntensity must be between 0.0 and 1.0"}
	}
	return nil
}

// Repository is Pulse's own preferences storage.
type Repository interface {
	// Get returns DefaultPreferences(userID) when no row exists yet -
	// no-preferences-set is the normal state for most users, not a
	// not-found error, mirroring Core's own QuietHours repository
	// convention.
	Get(ctx context.Context, userID string) (Preferences, error)
	Set(ctx context.Context, p Preferences) (Preferences, error)
}

// QuietHours mirrors Core's notifications.QuietHours shape (duplicated,
// not imported - this codebase's consumer-defined-interface
// convention). StartMinute/EndMinute are minutes since local midnight
// in Timezone.
type QuietHours struct {
	Timezone    string
	StartMinute int
	EndMinute   int
	Enabled     bool
}

type SetQuietHoursInput struct {
	Timezone    string
	StartMinute int
	EndMinute   int
	Enabled     bool
}

func (in SetQuietHoursInput) Validate() error {
	if strings.TrimSpace(in.Timezone) == "" {
		return &ValidationError{Message: "timezone is required"}
	}
	if in.StartMinute < 0 || in.StartMinute >= 1440 || in.EndMinute < 0 || in.EndMinute >= 1440 {
		return &ValidationError{Message: "startMinute and endMinute must be between 0 and 1439"}
	}
	return nil
}

// CoreQuietHours is Core's real, already-live notification-preferences
// capability - the caller's own per-caller *coresdk.Client is already
// scoped to their identity (there is no cross-user read here, exactly
// like Core's own HTTP handler only ever resolves the caller's own
// window), so neither method takes a userID.
type CoreQuietHours interface {
	Get(ctx context.Context) (QuietHours, error)
	Set(ctx context.Context, in SetQuietHoursInput) (QuietHours, error)
}

// Mute mirrors Core's trustsafety.Mute shape.
type Mute struct {
	ID          string
	MutedUserID string
	CreatedAt   time.Time
}

// CoreMutes is Core's real, already-live, platform-wide mute
// capability (trustsafety.Mute) - deliberately not duplicated as
// Pulse-owned storage. Note this is a self-scoped surface only ("who
// have I muted"); Core exposes no way to check "does user X mute me,"
// so delivery-time mute suppression (Architecture Audit's
// AUTHORIZATION MODEL item 7) is a named, deferred gap - see this
// module's README.
type CoreMutes interface {
	Mute(ctx context.Context, mutedUserID string) (Mute, error)
	List(ctx context.Context) ([]Mute, error)
	Unmute(ctx context.Context, mutedUserID string) error
}
