// Package analytics implements Phase 23: a generic analytics event
// envelope, ingested through core-api and flushed downstream by a
// worker pipeline (backend/worker/internal/analyticspipeline) - never
// queried for analytics purposes from this platform's own operational
// Postgres, per the roadmap's own explicit "operational databases must
// not become analytics databases" constraint.
package analytics

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Event is the roadmap's own named envelope shape, field for field:
// event_name, user_id, anonymous_id, app_id, session_id, timestamp,
// properties, context. EventName is free-form and product-defined
// (e.g. "screen_viewed", "purchase_completed"), the same convention as
// every other Type/Name-shaped field in this repo - this package has no
// fixed catalog of event names to validate against.
type Event struct {
	ID          string
	EventName   string
	UserID      string
	AnonymousID string
	AppID       string
	SessionID   string
	OccurredAt  time.Time
	Properties  map[string]any
	Context     map[string]any
	IngestedAt  time.Time
	FlushedAt   *time.Time
	BatchRef    string
}

// TrackInput is what a caller submits - IngestedAt/FlushedAt/BatchRef
// are always server-set, never accepted from the request body.
type TrackInput struct {
	EventName   string
	UserID      string
	AnonymousID string
	AppID       string
	SessionID   string
	OccurredAt  time.Time
	Properties  map[string]any
	Context     map[string]any
}

// Validate follows the same "either identity anchor works" rule real
// analytics specs use (Segment's track call requires userId OR
// anonymousId): an event about literally nobody isn't useful, but this
// package still doesn't require a verified platform identity the way
// every other module's requireUser does - see README.md for why
// tracking is deliberately open, unlike the rest of this platform.
func (in TrackInput) Validate() error {
	if strings.TrimSpace(in.EventName) == "" {
		return &ValidationError{"eventName is required"}
	}
	if strings.TrimSpace(in.AppID) == "" {
		return &ValidationError{"appId is required"}
	}
	if strings.TrimSpace(in.UserID) == "" && strings.TrimSpace(in.AnonymousID) == "" {
		return &ValidationError{"either userId or anonymousId is required"}
	}
	return nil
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrForbidden   = errors.New("not permitted to perform this analytics action")
	ErrRateLimited = errors.New("too many events - try again later")
)

// Repository is the storage-agnostic boundary. There is deliberately no
// query method here beyond a small, admin-only debug listing (see
// Service.ListRecent) - this package is not where analytics questions
// get answered.
type Repository interface {
	RecordBatch(ctx context.Context, events []Event) error
	ListRecent(ctx context.Context, limit int) ([]Event, error)
}
