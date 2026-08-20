// Package mood is Pulse's passive emotional state layer (product spec
// §22-27, Phase 8): "How am I feeling today?" without a status sentence
// - a single visual symbol, an audience, and a day-boundary expiry, not
// a message thread. Today's Mood is a singleton per user (one current
// Mood, replaced wholesale by the next Set - spec §26's "do not keep
// stale moods indefinitely as active state," and §26's own "historical
// Mood storage is a separate product decision" explicitly rules a full
// history out of this phase). Responding to a Mood is deliberately not
// a new capability here: spec §27 lists Pulse and Knock themselves as
// the non-verbal response - both already exist (Phase 4, Phase 7) and
// work against any connected user ID, so viewing a Mood and Pulsing or
// Knocking its owner needs no new backend surface, only a mobile UI
// affordance.
package mood

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Emoji string

// The predefined visual set spec §22 names - Mood is intentionally
// closed-vocabulary (never free text, spec §23's whole design
// philosophy), so this is validated server-side, not merely a UI
// suggestion.
const (
	EmojiSun   Emoji = "☀️"
	EmojiRain  Emoji = "🌧️"
	EmojiMoon  Emoji = "🌙"
	EmojiFire  Emoji = "🔥"
	EmojiWave  Emoji = "🌊"
	EmojiHug   Emoji = "🫂"
	EmojiHeart Emoji = "❤️"
	EmojiSleep Emoji = "💤"
)

func (e Emoji) valid() bool {
	switch e {
	case EmojiSun, EmojiRain, EmojiMoon, EmojiFire, EmojiWave, EmojiHug, EmojiHeart, EmojiSleep:
		return true
	default:
		return false
	}
}

type Audience string

// All six of spec §25's audience values are implemented. Five resolve
// from capabilities Pulse already had going into Phase 8 (bond's own
// active-partner record, pulse-connections' Friend/Close-Friend
// classification, or a caller-supplied user list); AudienceSelectedCircles
// (Phase 9) resolves against a real Core Group via pulse-connections'
// own Circles wrapper (internal/pulseconnections' CoreGroups) - the
// integration Phase 8 explicitly deferred until Circles existed.
const (
	AudiencePartnerOnly     Audience = "partner_only"
	AudienceCloseFriends    Audience = "close_friends"
	AudienceAllConnections  Audience = "all_connections"
	AudienceCustomUsers     Audience = "custom_users"
	AudiencePrivate         Audience = "private"
	AudienceSelectedCircles Audience = "selected_circles"
)

func (a Audience) valid() bool {
	switch a {
	case AudiencePartnerOnly, AudienceCloseFriends, AudienceAllConnections, AudienceCustomUsers, AudiencePrivate, AudienceSelectedCircles:
		return true
	default:
		return false
	}
}

// Mood is the owner's current Today's Mood. AllowedViewerIDs is the
// audience *resolved once, at Set*, from the owner's own real
// connections/bond state at that moment - never re-derived per viewer.
// This mirrors pulse-interactions' DeliveryMode ("decided once ... never
// re-decided mid-interaction"): a later reclassification, unfriend, or
// bond change doesn't retroactively change who can already see a Mood
// that's already been set: the next Set recomputes the audience fresh.
// It also sidesteps a real access-control problem this phase's Get
// handler would otherwise have - the viewer's own request token proves
// who *they* are, never who the *owner* has classified as a close
// friend, so that resolution can only honestly happen under the owner's
// own credentials, at Set time.
type Mood struct {
	UserID           string
	Emoji            Emoji
	Audience         Audience
	AllowedViewerIDs []string
	// CircleID is only meaningful for AudienceSelectedCircles - kept
	// alongside the already-resolved AllowedViewerIDs purely for the
	// owner's own management view (toMoodResponse), the same way
	// CustomUserIDs is echoed back for AudienceCustomUsers; CanView
	// itself never reads it, only AllowedViewerIDs.
	CircleID  *string
	CreatedAt time.Time
	ExpiresAt time.Time
	UpdatedAt time.Time
}

// CanView reports whether viewerID may see this Mood right now. The
// owner can always see their own Mood (until it expires, same as anyone
// else - spec §26 never carves out an owner exception to "do not keep
// stale moods indefinitely as active state"). AudiencePrivate never
// grants anyone but the owner, regardless of AllowedViewerIDs.
func (m Mood) CanView(viewerID string, now time.Time) bool {
	if now.After(m.ExpiresAt) {
		return false
	}
	if viewerID == m.UserID {
		return true
	}
	if m.Audience == AudiencePrivate {
		return false
	}
	for _, id := range m.AllowedViewerIDs {
		if id == viewerID {
			return true
		}
	}
	return false
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var ErrNotFound = errors.New("mood not found")

// ConnectionRef is pulse-connections' own Connection, narrowed to what
// audience resolution needs - duplicated rather than imported (this
// codebase's consumer-defined-interface convention: every module
// defines its own local view of another module's data, even when both
// live in the same binary).
type ConnectionRef struct {
	OtherUserID    string
	Status         string // mirrors Core relationships.Status: pending/active/blocked/ended
	Classification string // "friend" or "close_friend", pulse-connections' own classification
}

// Connections resolves the caller's own active connections plus their
// own Friend/Close-Friend classification of each - satisfied by an
// adapter over pulse-connections' real *Service (same binary, same
// database), never a duplicate copy of that data. Only ever called with
// the *owner's* own caller context, at Set time - see Mood's own doc
// comment for why.
type Connections interface {
	ListMine(ctx context.Context, callerID string) ([]ConnectionRef, error)
}

// Bond resolves the caller's own current active Bond partner, if any -
// satisfied directly by bond's real *Service.MyActiveBond (identical
// shape, no adapter needed, matching this codebase's "satisfied
// directly by *authz.Service" precedent for same-process dependencies).
type Bond interface {
	MyActiveBond(ctx context.Context, callerID string) (BondRef, error)
}

// BondRef mirrors bond.Bond, narrowed to what audience resolution
// needs - duplicated rather than imported, same reasoning as
// ConnectionRef.
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

// ErrNoBond is Bond's answer to "this caller doesn't currently hold an
// active Bond" - mirrors bond.ErrNotFound's meaning without importing
// the bond package's error value directly (this file only depends on
// bond's Service through the Bond interface above).
var ErrNoBond = errors.New("no active bond")

// Circles resolves the caller's own real Circle membership - satisfied
// by an adapter over pulse-connections' real Circle-wrapping *Service
// methods (internal/mood/pulsemodules), never a duplicated copy of
// Core's real groups data. ListMembers returning an error (the caller
// isn't a member of circleID, or it doesn't exist) is treated by Set as
// a validation failure - see resolveAudience's AudienceSelectedCircles
// branch - since Core's own groups.ListMembers already enforces
// membership server-side (see backend/core-api/internal/groups'
// requireMember), so a non-member referencing someone else's Circle ID
// fails honestly rather than silently resolving to an empty audience.
type Circles interface {
	ListMembers(ctx context.Context, callerID, circleID string) ([]string, error)
}

// Analytics records mood_set/mood_cleared (spec §103's "analytics
// exists") via Core's real analytics ingest - a fixed, service-level
// dependency (same shape as pulse-interactions' own Analytics), unlike
// Connections above which is resolved per-caller.
type Analytics interface {
	Track(ctx context.Context, eventName, userID string, properties map[string]any) error
}

// Repository is Pulse's own Mood storage - a singleton row per user,
// upserted by Set. Get and ListVisible both apply the expiry filter
// server-side (spec §26: an expired Mood is functionally gone, not
// merely marked "expired" and still returned) - never a background
// sweep job this phase, matching the spec's own allowance that
// historical/cleanup semantics are a separate decision.
type Repository interface {
	Set(ctx context.Context, m Mood) (Mood, error)
	// Get returns ErrNotFound if the user has no Mood, or their Mood has
	// already expired.
	Get(ctx context.Context, userID string) (Mood, error)
	Clear(ctx context.Context, userID string) error
}

// SetInput is Set's request shape. CustomUserIDs is only meaningful
// (and only read) when Audience is AudienceCustomUsers; CircleID only
// when Audience is AudienceSelectedCircles.
type SetInput struct {
	Emoji         Emoji
	Audience      Audience
	CustomUserIDs []string
	CircleID      string
}

func (in SetInput) Validate() error {
	if !in.Emoji.valid() {
		return &ValidationError{Message: "emoji must be one of the predefined Mood symbols"}
	}
	if !in.Audience.valid() {
		return &ValidationError{Message: "audience must be one of: partner_only, close_friends, all_connections, custom_users, private, selected_circles"}
	}
	if in.Audience == AudienceCustomUsers && len(in.CustomUserIDs) == 0 {
		return &ValidationError{Message: "customUserIds is required when audience is custom_users"}
	}
	if in.Audience != AudienceCustomUsers && len(in.CustomUserIDs) > 0 {
		return &ValidationError{Message: "customUserIds is only meaningful when audience is custom_users"}
	}
	if in.Audience == AudienceSelectedCircles && strings.TrimSpace(in.CircleID) == "" {
		return &ValidationError{Message: "circleId is required when audience is selected_circles"}
	}
	if in.Audience != AudienceSelectedCircles && in.CircleID != "" {
		return &ValidationError{Message: "circleId is only meaningful when audience is selected_circles"}
	}
	return nil
}
