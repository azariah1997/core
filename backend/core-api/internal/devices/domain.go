// Package devices implements per-user device/session tracking: which
// installs of which apps a user is logged in on, so they can see and
// revoke them (a lost phone, a shared computer). A device's session_status
// stands in for a first-class Session concept - nothing here yet needs
// more than one live session per device.
package devices

import (
	"context"
	"errors"
	"strings"
	"time"
)

type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionRevoked SessionStatus = "revoked"
)

type Device struct {
	ID             string
	UserID         string
	ClientDeviceID string
	Platform       string
	OSVersion      string
	AppVersion     string
	Locale         string
	Timezone       string
	// HasPushToken, never the token itself: push tokens are never returned
	// once registered.
	HasPushToken  bool
	SessionStatus SessionStatus
	LastActiveAt  time.Time
	CreatedAt     time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var ErrNotFound = errors.New("device not found")

// RegisterInput registers or re-registers a device. ClientDeviceID is
// generated once by the client and persisted locally - it's what makes
// repeated calls from the same install an upsert (updating profile fields
// and LastActiveAt) instead of creating duplicate rows.
type RegisterInput struct {
	ClientDeviceID string
	Platform       string
	OSVersion      string
	AppVersion     string
	Locale         string
	Timezone       string
	PushToken      string
}

func (in RegisterInput) Validate() error {
	if strings.TrimSpace(in.ClientDeviceID) == "" {
		return &ValidationError{"clientDeviceId is required"}
	}
	if strings.TrimSpace(in.Platform) == "" {
		return &ValidationError{"platform is required"}
	}
	return nil
}

// Repository is the storage-agnostic boundary. List and Revoke are always
// scoped to a userID, not just a device ID, so a caller can never see or
// revoke another user's device even if it guessed the ID.
type Repository interface {
	Register(ctx context.Context, userID string, in RegisterInput) (Device, error)
	List(ctx context.Context, userID string) ([]Device, error)
	Revoke(ctx context.Context, userID, deviceID string) error
	// RevokeAll is Phase 20's addition - used only by the privacy
	// deletion participant (internal/api/privacy_adapters.go), never by
	// an HTTP route: revoking every one of a user's devices in one call
	// is exactly the kind of system-level action a normal caller has no
	// reason to trigger directly.
	RevokeAll(ctx context.Context, userID string) error
}
