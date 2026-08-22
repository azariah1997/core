// Package livetouch is Pulse's flagship synchronous two-way touch
// feature (product spec §21, Phase 10): "when two users are
// simultaneously active, User A touches -> User B feels it, and back."
// Live Touch is gated on an active Partner Bond (spec §7: Bond
// "unlocks... Live Touch"), never merely Friend-or-Bond the way Pulse
// and Knock are - a genuinely different authorization shape, which is
// why this is its own module rather than a third pulse-interactions
// Type the way Knock was.
//
// This module owns the *session lifecycle* only - invite, accept,
// decline, end, with a durable, server-computed duration for the parts
// spec's own Definition of Done (§104) can honestly verify offline
// (session creation, acceptance, completion). The touch-start/touch-stop
// events themselves are deliberately never persisted here and never
// proxied through pulse-api: once a session is active, both
// participants subscribe directly to a per-session realtime-gateway
// channel ("pulse:live-touch:{sessionId}") and exchange touch events
// client-to-client through the hub's existing real publish/subscribe
// relay (backend/realtime-gateway/internal/ws) - the lowest-latency
// path already available on this platform, and exactly what spec §21's
// "keep latency as low as reasonably possible" asks for. The session ID
// itself (a cryptographically random UUID) is the only access control
// realtime-gateway's generic channel protocol offers today - a real,
// honestly-documented platform limitation (see apps/pulse/docs/
// ARCHITECTURE_AUDIT.md's Risks), not a stronger per-channel ACL.
package livetouch

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusInvited Status = "invited"
	StatusActive  Status = "active"
	StatusEnded   Status = "ended"
)

type EndReason string

const (
	EndReasonDeclined EndReason = "declined"
	EndReasonEnded    EndReason = "ended"
	// EndReasonTimeout covers both an invite never accepted in time and
	// an active session abandoned with no End call - both detected
	// lazily, on the next read, never a background sweep job (same
	// pattern mood's own expiry already established). This is an honest
	// proxy for "disconnect handled," not true instant cross-device
	// disconnect detection - realtime-gateway's hub doesn't broadcast
	// channel-peer-disconnected events today (see this package's own
	// header comment).
	EndReasonTimeout EndReason = "timeout"
)

// inviteTimeout is how long an invite may sit unaccepted before it's
// treated as expired. abandonedTimeout is a much longer ceiling on an
// active session nobody ever explicitly ended - a safety net against a
// session record staying "active" forever after both apps were simply
// closed, not a substitute for the mobile client's own End call on
// disconnect.
const (
	inviteTimeout    = 2 * time.Minute
	abandonedTimeout = 30 * time.Minute
)

// Channel is the realtime-gateway channel name both participants
// subscribe to once a session is active - never persisted separately,
// always derivable from the session ID alone.
func (s Session) Channel() string { return "pulse:live-touch:" + s.ID }

type Session struct {
	ID           string
	InitiatorID  string
	ReceiverID   string
	Status       Status
	EndReason    *EndReason
	DeliveryMode DeliveryMode
	InvitedAt    time.Time
	AcceptedAt   *time.Time
	EndedAt      *time.Time
	DurationMs   *int
	UpdatedAt    time.Time
}

func (s Session) otherUser(callerID string) string {
	if s.InitiatorID == callerID {
		return s.ReceiverID
	}
	return s.InitiatorID
}

func (s Session) isParticipant(userID string) bool {
	return s.InitiatorID == userID || s.ReceiverID == userID
}

// DeliveryMode describes how the *invite* was delivered - live (the
// receiver's device got a real-time push the instant it was sent) or
// push (a durable notification, since the receiver wasn't connected to
// realtime-gateway at invite time). This is never a claim about whether
// the touch session itself will work - that depends on both
// participants actually being connected to realtime-gateway once the
// session goes active, which the channel subscribe step itself
// naturally enforces (an offline device simply receives nothing).
type DeliveryMode string

const (
	DeliveryLive DeliveryMode = "live"
	DeliveryPush DeliveryMode = "push"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("live touch session not found")
	ErrForbidden = errors.New("not permitted to perform this action on this session")
	// ErrNotBonded is Live Touch's own authorization gate (spec §7:
	// Bond "unlocks... Live Touch") - stricter than pulse-interactions'
	// Friend-or-Bond ErrNotConnected, since only the caller's real,
	// current active Bond partner may be invited.
	ErrNotBonded         = errors.New("live touch is only available with your current active bond partner")
	ErrInvalidTransition = errors.New("session is not in a state that allows this action")
	ErrRateLimited       = errors.New("rate limit exceeded")
)

// BondRef mirrors bond.Bond and mood.BondRef, narrowed to what Live
// Touch's own authorization needs - duplicated rather than imported
// (this codebase's consumer-defined-interface convention).
type BondRef struct {
	UserAID string
	UserBID string
}

func (b BondRef) otherUser(callerID string) string {
	if b.UserAID == callerID {
		return b.UserBID
	}
	return b.UserAID
}

// ErrNoBond mirrors mood.ErrNoBond's meaning.
var ErrNoBond = errors.New("no active bond")

// Bond resolves the caller's own current active Bond partner - satisfied
// directly by bond's real *Service.MyActiveBond (identical shape, no
// adapter needed beyond translating the not-found case, matching this
// codebase's "satisfied directly by *authz.Service" precedent for
// same-process dependencies).
type Bond interface {
	MyActiveBond(ctx context.Context, callerID string) (BondRef, error)
}

// Presence answers "is this user connected right now" - resolved
// per-caller against realtime-gateway's real GET /v1/presence/{userId},
// the same pattern pulse-interactions' own Presence interface uses, to
// decide the invite's DeliveryMode.
type Presence interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
}

// Realtime delivers a best-effort live push announcing session
// lifecycle events (invited/accepted/declined/ended) - never the touch
// events themselves, which flow over the session's own channel instead
// (see this package's header comment).
type Realtime interface {
	ToUser(ctx context.Context, userID string, payload []byte) error
}

// Notifier requests a durable push notification via Core's real
// notifications.Send when an invite can't be delivered live - same
// shape as pulse-interactions' own Notifier.
type Notifier interface {
	Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error
}

// Analytics records durable session-lifecycle events, including the one
// genuinely server-measurable latency this phase can honestly claim:
// invite-to-accept time (spec §104's "latency measured") - per-touch-event
// round-trip latency happens entirely client-to-client over the
// session's realtime channel and is a mobile-side instrumentation
// concern this phase doesn't add.
type Analytics interface {
	Track(ctx context.Context, eventName, userID string, properties map[string]any) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Repository is Pulse's own Live Touch session storage.
type Repository interface {
	Create(ctx context.Context, s Session) (Session, error)
	Get(ctx context.Context, id string) (Session, error)
	Accept(ctx context.Context, id string, acceptedAt time.Time) (Session, error)
	End(ctx context.Context, id string, endedAt time.Time, reason EndReason) (Session, error)
}
