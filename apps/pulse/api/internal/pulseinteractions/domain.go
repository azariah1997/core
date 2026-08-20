// Package pulseinteractions is Pulse's core mechanic (product spec
// §13-17): press-and-hold Pulse, felt live on the other end when the
// receiver is connected (Phase 4), falling back to a durable push
// notification when they're not (Phase 5, spec §4.2). Which path a
// given Pulse takes is decided once, at Start, from a real presence
// check against realtime-gateway - never guessed and never re-decided
// mid-interaction. The PUSH_REQUESTED/PUSH_SENT/OPENED sub-states from
// spec §16 are still not implemented - DeliveryMode (live vs. push) is
// the granularity this phase actually needs and can honestly confirm.
package pulseinteractions

import (
	"context"
	"errors"
	"time"
)

type Type string

// TypePulse and TypeKnock share exactly one state machine and one
// table - CustomSignal (Phase 11), LiveTouch (Phase 10), and
// MoodResponse (Phase 8) will too. Start/Stop's actual logic (presence
// check, delivery-mode decision, authorization) is 100% shared; only
// the realtime event name, notification content, and analytics event
// differ per Type (see Stop's own type switch) - this is the
// "extensible architecture" Phase 7 was explicitly asked to leave
// behind for Phase 11's custom signals.
const (
	TypePulse Type = "pulse"
	TypeKnock Type = "knock"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusStarted   Status = "started"
	StatusCompleted Status = "completed"
)

type DeliveryMode string

const (
	// DeliveryLive: the receiver was online (real presence check) when
	// Start was called - a best-effort real-time push at Start and at
	// Stop. rtbus.ToUser has no delivery confirmation (it fans out to
	// Redis regardless of whether anyone is actually connected), so
	// this never claims a "live_delivered" status without real
	// acknowledgment - "live" describes the attempted path, not a
	// confirmed outcome.
	DeliveryLive DeliveryMode = "live"
	// DeliveryPush: the receiver was offline at Start - no live attempt
	// is made at all; a single durable push notification (spec §4.2)
	// is requested once, at Stop, carrying the real final duration.
	DeliveryPush DeliveryMode = "push"
)

// RelationshipType values pulse-interactions checks for an active
// connection. Pulse is allowed between Friends (spec §8) and not
// restricted to Bond partners the way Live Touch (Phase 10) is.
const (
	FriendRelationshipType = "pulse_friend"
	BondRelationshipType   = "pulse_bond"
)

type Interaction struct {
	ID         string
	Type       Type
	SenderID   string
	ReceiverID string
	StartedAt  *time.Time
	EndedAt    *time.Time
	DurationMs *int
	// InResponseToID links a Pulse Back (spec §17) to the original
	// interaction it's reciprocating - nil for an ordinary Pulse.
	InResponseToID *string
	// Pattern is only meaningful for TypeKnock this phase - a name from
	// a small predefined set (spec §18), not yet the free-form
	// tap/hold/pause segment array Phase 11's Custom Signals will add.
	// A plain nullable string, not KnockPattern, so this same field can
	// hold whatever later phases need without another migration.
	Pattern      *string
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

// Presence answers "is this user connected right now" - resolved
// per-caller (the sender's own token, forwarded to realtime-gateway's
// real GET /v1/presence/{userId}), never a fixed service-level
// credential, the same pattern CoreRelationships uses.
type Presence interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
}

// Notifier requests a durable push notification via Core's real
// notifications.Send (spec §71: "Pulse must not call APNs/FCM
// directly"), resolved per-caller so the send is attributed to the
// real sender (see notifications/README.md for why cross-user Send is
// now allowed at all). One generic method, not one per interaction Type
// (NotifyPulseReceived, then NotifyKnockReceived, then a third for
// Phase 8's Mood Response...) - category/title/body are the caller's
// job to choose, matching Stop's own type switch. Content is
// deliberately generic ("New Pulse"/"Knock", spec §72's Private tier
// default) rather than naming the sender - that needs pulse-profile's
// handle, a cross-module dependency not worth taking on yet, and
// content-level privacy preferences are Phase 13's job.
type Notifier interface {
	Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error
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
	// Start also persists deliveryMode - decided once, from a real
	// presence check, and never re-decided mid-interaction.
	Start(ctx context.Context, id string, startedAt time.Time, deliveryMode DeliveryMode) (Interaction, error)
	// Stop computes DurationMs server-side from the row's own
	// StartedAt, never trusting a client-submitted value (spec §15,
	// §78 - "server timestamps are truth").
	Stop(ctx context.Context, id string, endedAt time.Time) (Interaction, error)
	ListForUser(ctx context.Context, userID string, limit int) ([]Interaction, error)
}

type CreateInput struct {
	ReceiverID      string
	ClientRequestID string
	// InResponseToID is set internally by PulseBack - never accepted
	// from a client request body, so a caller can't forge a fake
	// reciprocation link (see http.go: only PulseBack's own handler
	// sets this, createHandler never reads it from the request).
	InResponseToID string
	// Pattern is set internally by Knock - see Interaction.Pattern.
	Pattern string
}

func (in CreateInput) Validate() error {
	if in.ReceiverID == "" {
		return &ValidationError{Message: "receiverId is required"}
	}
	return nil
}

// KnockPattern is a name from the small predefined set spec §18 names
// (••, •••, — •, • — •) - not yet the free-form segment array Phase
// 11's Custom Signals will add, but a plain string field on Interaction
// (not this type) so that later addition needs no new column.
type KnockPattern string

const (
	KnockPatternDoubleTap      KnockPattern = "double_tap"       // •• - spec §18's own default example
	KnockPatternTripleTap      KnockPattern = "triple_tap"       // •••
	KnockPatternLongShort      KnockPattern = "long_short"       // — •
	KnockPatternShortLongShort KnockPattern = "short_long_short" // • — •
)

func (p KnockPattern) valid() bool {
	switch p {
	case KnockPatternDoubleTap, KnockPatternTripleTap, KnockPatternLongShort, KnockPatternShortLongShort:
		return true
	default:
		return false
	}
}

type KnockInput struct {
	ReceiverID string
	// Pattern defaults to KnockPatternDoubleTap when empty.
	Pattern KnockPattern
}
