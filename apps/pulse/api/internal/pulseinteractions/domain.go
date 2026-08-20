// Package pulseinteractions is Pulse's core mechanic (product spec
// §13-17, Phase 4): press-and-hold Pulse, felt live on the other end.
// This phase implements the live path only - PulseStart/PulseStop
// delivered over Core's real realtime infrastructure while the
// receiver is connected. Offline/background push fallback (spec §4.2)
// is deliberately deferred to Phase 5, matching the roadmap's own
// phase split; DeliveryMode "push"/"deferred" and the
// PUSH_REQUESTED/PUSH_SENT/OPENED states from spec §16 are not
// implemented yet.
package pulseinteractions

import (
	"context"
	"errors"
	"time"
)

type Type string

// TypePulse is the only interaction type this phase creates. Knock
// (Phase 7), CustomSignal (Phase 11), LiveTouch (Phase 10), and
// MoodResponse (Phase 8) reuse this same state machine and table in
// later phases, not a parallel one.
const TypePulse Type = "pulse"

type Status string

const (
	StatusCreated   Status = "created"
	StatusStarted   Status = "started"
	StatusCompleted Status = "completed"
)

type DeliveryMode string

const (
	// DeliveryLive is this phase's only mode: a best-effort real-time
	// push while the interaction is live. rtbus.ToUser has no delivery
	// confirmation (it fans out to Redis regardless of whether anyone
	// is actually connected), so this phase deliberately does not claim
	// a "live_delivered" status without real acknowledgment - see
	// VALIDATION-style notes for this phase's honest scope.
	DeliveryLive DeliveryMode = "live"
)

// RelationshipType values pulse-interactions checks for an active
// connection. Pulse is allowed between Friends (spec §8) and not
// restricted to Bond partners the way Live Touch (Phase 10) is.
const (
	FriendRelationshipType = "pulse_friend"
	BondRelationshipType   = "pulse_bond"
)

type Interaction struct {
	ID           string
	Type         Type
	SenderID     string
	ReceiverID   string
	StartedAt    *time.Time
	EndedAt      *time.Time
	DurationMs   *int
	DeliveryMode DeliveryMode
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (i Interaction) otherUser(callerID string) string {
	if i.SenderID == callerID {
		return i.ReceiverID
	}
	return i.SenderID
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("interaction not found")
	ErrForbidden = errors.New("not permitted to perform this action on this interaction")
	// ErrNotConnected covers both "no connection exists yet" and
	// "blocked" - the caller-facing message differs (see
	// writeDomainError) but both are a hard no server-side, per the
	// authorization model in apps/pulse/docs/ARCHITECTURE_AUDIT.md.
	ErrNotConnected = errors.New("no active connection exists between these users")
	ErrBlocked      = errors.New("this user cannot be reached")
	ErrRateLimited  = errors.New("rate limit exceeded")
	// ErrInvalidTransition covers calling start/stop out of order -
	// e.g. stopping an interaction that was never started, or starting
	// one twice. Mirrors Core relationships' own ErrInvalidTransition
	// naming and 409 mapping.
	ErrInvalidTransition = errors.New("interaction is not in a state that allows this action")
)

// RelationshipRef mirrors bond/pulse-connections' own type of the same
// name - duplicated rather than shared, matching this codebase's
// consumer-defined-interface convention.
type RelationshipRef struct {
	RequesterID string
	TargetID    string
	Status      string
}

// CoreRelationships is the one Core capability this module needs beyond
// realtime/analytics - just enough to answer "are these two users
// connected, and not blocked" (spec's authorization model). Never the
// full request/accept/decline surface pulse-connections/bond need,
// since pulse-interactions never creates or mutates a relationship.
type CoreRelationships interface {
	ListMine(ctx context.Context, relType string) ([]RelationshipRef, error)
}

// Realtime is the live-delivery mechanism - satisfied directly by
// platformkit/rtbus.Publisher.ToUser in production (a fixed
// service-level dependency, constructed once at startup with the
// shared Redis client, unlike CoreRelationships which is resolved
// per-caller) and a fake in tests.
type Realtime interface {
	ToUser(ctx context.Context, userID string, payload []byte) error
}

// Analytics records the durable pulse_completed event (spec §79) via
// Core's real analytics ingest - satisfied by a thin adapter over
// coresdk.Client.Do in production.
type Analytics interface {
	Track(ctx context.Context, eventName, userID string, properties map[string]any) error
}

// RateLimiter is satisfied directly by platformkit/ratelimit.Limiter -
// the same reused-not-reimplemented pattern trustsafety's own
// RateLimiter interface already established for this exact type.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Repository is Pulse's own interaction storage.
type Repository interface {
	// Create is idempotent when clientRequestID is non-empty: a retry
	// with the same (senderID, clientRequestID) returns the original
	// row rather than creating a duplicate (spec §76).
	Create(ctx context.Context, i Interaction, clientRequestID string) (Interaction, error)
	Get(ctx context.Context, id string) (Interaction, error)
	Start(ctx context.Context, id string, startedAt time.Time) (Interaction, error)
	// Stop computes DurationMs server-side from the row's own
	// StartedAt, never trusting a client-submitted value (spec §15,
	// §78 - "server timestamps are truth").
	Stop(ctx context.Context, id string, endedAt time.Time) (Interaction, error)
	ListForUser(ctx context.Context, userID string, limit int) ([]Interaction, error)
}

type CreateInput struct {
	ReceiverID      string
	ClientRequestID string
}

func (in CreateInput) Validate() error {
	if in.ReceiverID == "" {
		return &ValidationError{Message: "receiverId is required"}
	}
	return nil
}
