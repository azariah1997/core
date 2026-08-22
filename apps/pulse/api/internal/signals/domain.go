// Package signals is Pulse's private custom touch-pattern storage
// (product spec §19-20, Phase 11): "users should eventually be able to
// communicate using patterns whose meaning exists only between them...
// the platform does not need to understand the semantic meaning. The
// pattern itself is the communication." This module owns the saved
// pattern *library* only - create/list/delete a bounded sequence of
// tap/hold/pause segments, bound to exactly one specific connection
// (spec §19's own example: the same "••" pattern means something
// different to different pairs, so a pattern only ever means one thing
// - the one relationship it was created for). Actually *sending* a
// signal - felt live or via push fallback, exactly like Pulse and Knock
// - is pulse-interactions' job (a fourth Type, TypeSignal), reusing its
// exact authorization/rate-limit/delivery machinery rather than
// duplicating it, the same reuse Knock already established in Phase 7 -
// this module never talks to realtime-gateway or Core's notifications
// itself.
package signals

import (
	"context"
	"errors"
	"strings"
	"time"
)

type SegmentType string

const (
	SegmentTap   SegmentType = "tap"
	SegmentHold  SegmentType = "hold"
	SegmentPause SegmentType = "pause"
)

func (t SegmentType) valid() bool {
	switch t {
	case SegmentTap, SegmentHold, SegmentPause:
		return true
	default:
		return false
	}
}

// Bounds are explicit, server-enforced-at-creation limits (Architecture
// Audit's Risk #4: "an unbounded segments[] array could be used to
// construct a very long or rapid vibration pattern... needs explicit
// bounds (max segment count, max total duration) enforced server-side
// at creation time, not just client-side UI limits"). minSegmentDurationMs
// rules out a degenerate zero/negative-duration segment being used to
// pack more taps than maxSegments would otherwise allow.
const (
	maxSegments          = 20
	minSegmentDurationMs = 20
	maxSegmentDurationMs = 3000
	maxTotalDurationMs   = 10000
	maxLabelLength       = 60
)

type Segment struct {
	Type       SegmentType
	DurationMs int
}

func (s Segment) validate() error {
	if !s.Type.valid() {
		return &ValidationError{Message: "segment type must be one of: tap, hold, pause"}
	}
	if s.DurationMs < minSegmentDurationMs || s.DurationMs > maxSegmentDurationMs {
		return &ValidationError{Message: "segment durationMs must be between 20 and 3000"}
	}
	return nil
}

// Signal is one saved pattern, bound to exactly one specific connection
// (spec §19) - never a global/reusable pattern sendable to anyone.
// Label is optional, owner-visible only, and never interpreted
// server-side (spec §20's own allowance for "optional labels" without
// the platform assigning them any semantic meaning).
type Signal struct {
	ID           string
	OwnerUserID  string
	TargetUserID string
	Label        string
	Segments     []Segment
	CreatedAt    time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("signal not found")
	ErrForbidden = errors.New("not permitted to perform this action on this signal")
	// ErrNotConnected mirrors pulse-interactions' own meaning -
	// duplicated rather than imported (this codebase's
	// consumer-defined-interface convention).
	ErrNotConnected = errors.New("no active connection exists between these users")
	ErrBlocked      = errors.New("this user cannot be reached")
)

const (
	FriendRelationshipType = "pulse_friend"
	BondRelationshipType   = "pulse_bond"
)

// RelationshipRef mirrors pulse-interactions' own type of the same name.
type RelationshipRef struct {
	RequesterID string
	TargetID    string
	Status      string
}

// CoreRelationships is the one Core capability this module needs -
// enough to answer "are these two users connected, and not blocked,"
// the exact same authorization model pulse-interactions' own
// checkConnected already establishes.
type CoreRelationships interface {
	ListMine(ctx context.Context, relType string) ([]RelationshipRef, error)
}

// Repository is Pulse's own signal storage.
type Repository interface {
	Create(ctx context.Context, s Signal) (Signal, error)
	Get(ctx context.Context, id string) (Signal, error)
	ListMine(ctx context.Context, ownerUserID string) ([]Signal, error)
	Delete(ctx context.Context, id string) error
}

type CreateInput struct {
	TargetUserID string
	Label        string
	Segments     []Segment
}

func (in CreateInput) Validate() error {
	if strings.TrimSpace(in.TargetUserID) == "" {
		return &ValidationError{Message: "targetUserId is required"}
	}
	if len(in.Label) > maxLabelLength {
		return &ValidationError{Message: "label must be 60 characters or fewer"}
	}
	if len(in.Segments) == 0 {
		return &ValidationError{Message: "at least one segment is required"}
	}
	if len(in.Segments) > maxSegments {
		return &ValidationError{Message: "at most 20 segments are allowed"}
	}
	total := 0
	for _, seg := range in.Segments {
		if err := seg.validate(); err != nil {
			return err
		}
		total += seg.DurationMs
	}
	if total > maxTotalDurationMs {
		return &ValidationError{Message: "total pattern duration must be 10000ms or less"}
	}
	return nil
}
