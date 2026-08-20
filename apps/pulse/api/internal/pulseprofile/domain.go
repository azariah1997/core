// Package pulseprofile is Pulse's thin extension of a Core Platform
// User - a public handle plus Pulse-specific visual/interaction
// preferences. It never duplicates Core's identity or user record; the
// caller's Core User ID is the only foreign reference it holds, and
// that ID is resolved by pulseauth against Core's own GET /v1/users/me,
// never re-derived locally.
package pulseprofile

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Profile is Pulse's per-user extension row.
type Profile struct {
	UserID      string // Core User.ID - the only link back to Core, by value
	Handle      string // Pulse-specific public handle, distinct from any Core display name
	VisualPrefs map[string]any
	PulsePrefs  map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrNotFound    = errors.New("pulse profile not found")
	ErrHandleTaken = errors.New("handle already in use")
	handlePattern  = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)
)

// ValidationError signals a client input problem, distinct from
// infrastructure failures - matching every Core module's own
// convention so a handler can map it to a 4xx response the same way.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type CreateInput struct {
	Handle string
}

func (in CreateInput) Validate() error {
	handle := strings.ToLower(strings.TrimSpace(in.Handle))
	if !handlePattern.MatchString(handle) {
		return &ValidationError{Message: "handle must be 3-24 lowercase letters, digits, or underscores"}
	}
	return nil
}

type UpdateInput struct {
	VisualPrefs map[string]any
	PulsePrefs  map[string]any
}

// Repository is the interface pulseprofile.Service depends on -
// consumer-defined, not provider-defined, so both postgres and memory
// implementations satisfy it without service.go ever importing either.
type Repository interface {
	Create(ctx context.Context, p Profile) (Profile, error)
	Get(ctx context.Context, userID string) (Profile, error)
	GetByHandle(ctx context.Context, handle string) (Profile, error)
	Update(ctx context.Context, userID string, in UpdateInput) (Profile, error)
}
