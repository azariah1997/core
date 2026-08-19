// Package remoteconfig is the platform's dynamic settings store -
// "separate configuration from feature flags" is the roadmap's own
// instruction distinguishing this from Phase 17's targeting-rule
// evaluation engine. This is a typed key/value store: limits, URLs, UI
// options, service behaviour, minimum versions, maintenance state - the
// roadmap's own examples of what a value might hold, not a fixed set of
// supported keys.
package remoteconfig

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Entry is the current, live value for one (AppID, Environment, Key).
// Value is arbitrary JSON (a string, number, bool, object, or array) -
// config values are too varied in shape (a URL is a string, a limit is a
// number, "UI options" could be anything) for a fixed schema.
type Entry struct {
	ID          string
	AppID       string
	Environment string // free-form, e.g. "production" - no built-in fallback between environments
	Key         string // product-defined, e.g. "checkout.maxRetries"
	Value       any
	Description string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Change is one row of the audit history "all changes must be
// auditable" requires - every write produces one, including deletes
// (NewValue nil). Never updated or deleted itself.
type Change struct {
	ID            string
	AppID         string
	Environment   string
	Key           string
	PreviousValue any // nil on the first-ever write for a key
	NewValue      any // nil means this change was a delete
	ChangedBy     string
	Reason        string
	ChangedAt     time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("config entry not found")
	ErrForbidden = errors.New("not permitted to manage remote configuration")
)

type SetInput struct {
	AppID       string
	Environment string
	Key         string
	Value       any
	Description string
	Reason      string
}

func (in SetInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Environment) == "":
		return &ValidationError{"environment is required"}
	case strings.TrimSpace(in.Key) == "":
		return &ValidationError{"key is required"}
	case in.Value == nil:
		return &ValidationError{"value is required"}
	}
	return nil
}

// Repository is the storage-agnostic boundary. Implementations write the
// Entry and its Change audit row atomically (same transactional-outbox-
// adjacent discipline as every other write-path Repository in this repo,
// even though this one doesn't need the outbox pattern itself - see
// postgres/repository.go).
type Repository interface {
	Set(ctx context.Context, changedBy string, in SetInput) (Entry, error)
	Get(ctx context.Context, appID, environment, key string) (Entry, error)
	List(ctx context.Context, appID, environment string) ([]Entry, error)
	Delete(ctx context.Context, changedBy, appID, environment, key, reason string) error
	History(ctx context.Context, appID, environment, key string) ([]Change, error)
}
