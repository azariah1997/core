// Package pulseconnections is Pulse's connection experience (product
// spec §2, Phase 2): find/invite, request, accept, decline, remove, and
// Friend vs Close Friend classification. The connection lifecycle
// itself is never duplicated here - every request/accept/decline/
// remove call is forwarded to Core's real relationships API (see
// CoreRelationships and internal/pulseconnections/core), scoped to
// Core's relationship Type "pulse_friend" so Pulse's rows never
// collide with any other product's use of the same generic graph.
// Only the Pulse-specific classification (Friend vs Close Friend) is
// Pulse's own data, keyed by relationship ID - never stuffed into
// Core's product-agnostic Metadata field, per AI_CONTEXT.md's
// non-negotiable rule.
package pulseconnections

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RelationshipType is the one Core relationships.Type Pulse's
// connection experience ever requests. Bond (Phase 3) uses a distinct
// type ("pulse_bond") so the two are never confused in Core's shared
// graph.
const RelationshipType = "pulse_friend"

// bondRelationshipType mirrors bond.RelationshipType - duplicated
// rather than imported (this codebase's consumer-defined-interface
// convention), needed only so checkConnected below can recognize a
// Bond as a valid connection too, the same two-type check
// pulse-interactions' own checkConnected already makes.
const bondRelationshipType = "pulse_bond"

type Classification string

const (
	ClassificationFriend      Classification = "friend"
	ClassificationCloseFriend Classification = "close_friend"
)

func (c Classification) valid() bool {
	switch c {
	case ClassificationFriend, ClassificationCloseFriend:
		return true
	default:
		return false
	}
}

// RelationshipRef is the subset of a Core Relationship pulse-connections
// needs - never the full Core type, so this package doesn't couple to
// Core's internal shape beyond what it actually uses.
type RelationshipRef struct {
	ID          string
	RequesterID string
	TargetID    string
	Status      string // mirrors Core relationships.Status verbatim: pending/active/blocked/ended
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Connection is pulse-connections' own view of a relationship, from the
// perspective of the caller viewing it - the Core row plus Pulse's own
// classification overlay merged in.
type Connection struct {
	RelationshipID string
	OwnerUserID    string // the caller this view belongs to
	OtherUserID    string
	Status         string
	Direction      string // "outgoing" (caller is RequesterID) or "incoming"
	Classification Classification
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound  = errors.New("connection not found")
	ErrForbidden = errors.New("not permitted to perform this action on this connection")
	// ErrNotConnected gates Circle membership (product spec §10-11: a
	// Circle should only ever contain people you're actually connected
	// to - "users must mutually connect before direct interactions are
	// possible," and Circles exist to control those interactions'
	// audience/permissions). Mirrors pulse-interactions' own
	// ErrNotConnected meaning.
	ErrNotConnected = errors.New("no active connection exists between these users")
)

// CoreRelationships is the subset of Core's real relationships API this
// module needs, scoped to a single already-authenticated caller -
// http.go builds one per request from pulseauth's resolved client,
// never a fixed service-level credential (see internal/pulseconnections/
// core for the real coresdk-backed implementation, and service_test.go
// for the in-memory fake used in tests).
type CoreRelationships interface {
	Request(ctx context.Context, targetUserID, relType string) (RelationshipRef, error)
	Accept(ctx context.Context, relationshipID string) (RelationshipRef, error)
	Decline(ctx context.Context, relationshipID string) (RelationshipRef, error)
	Remove(ctx context.Context, relationshipID string) error
	ListMine(ctx context.Context, relType string) ([]RelationshipRef, error)
}

// Circle is Pulse's view of a Core Group (product spec §10: "Closest
// Friends, Family, University, Work Friends, Gaming Friends, Custom") -
// Circles are never a Pulse-owned table; Core's real `groups` capability
// (backend/core-api/internal/groups) already owns exactly this shape,
// so Pulse only ever wraps it (see CoreGroups), scoped to Pulse's own
// AppID.
type Circle struct {
	ID        string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CircleMember mirrors Core's GroupMember, narrowed to what Pulse
// needs.
type CircleMember struct {
	UserID    string
	Role      string
	IsManager bool
	CreatedAt time.Time
}

// CoreGroups is the subset of Core's real groups API Circles needs,
// scoped to a single already-authenticated caller (same per-caller
// pattern as CoreRelationships) - see internal/pulseconnections/core
// for the real coresdk-backed implementation.
type CoreGroups interface {
	Create(ctx context.Context, name string) (Circle, error)
	// ListMine returns every Circle the caller belongs to, already
	// filtered to Pulse's own AppID - Core's real ListMine has no
	// appId parameter (it returns every group the caller is in, across
	// every application), so that filtering happens in the adapter,
	// never here.
	ListMine(ctx context.Context) ([]Circle, error)
	ListMembers(ctx context.Context, circleID string) ([]CircleMember, error)
	AddMember(ctx context.Context, circleID, userID string) (CircleMember, error)
	RemoveMember(ctx context.Context, circleID, userID string) error
}

type CreateCircleInput struct {
	Name string
}

func (in CreateCircleInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return &ValidationError{Message: "name is required"}
	}
	return nil
}

// ClassificationRepository is the Pulse-owned storage boundary for the
// Friend/Close-Friend overlay - consumer-defined, satisfied by both
// postgres and memory implementations.
type ClassificationRepository interface {
	Set(ctx context.Context, relationshipID, ownerUserID string, c Classification) error
	// Get defaults to ClassificationFriend when nothing has been set -
	// every active connection starts as a plain Friend until explicitly
	// upgraded, matching product spec §9 ("not assume everyone sees
	// everything" implies the reverse too: nothing is close by default).
	Get(ctx context.Context, relationshipID, ownerUserID string) (Classification, error)
	// ListForOwner returns every classification the owner has ever set,
	// keyed by relationship ID, for bulk-merging into a connection list.
	ListForOwner(ctx context.Context, ownerUserID string) (map[string]Classification, error)
}

type RequestInput struct {
	TargetUserID string
}

func (in RequestInput) Validate() error {
	if strings.TrimSpace(in.TargetUserID) == "" {
		return &ValidationError{Message: "targetUserId is required"}
	}
	return nil
}

type SetClassificationInput struct {
	Classification Classification
}

func (in SetClassificationInput) Validate() error {
	if !in.Classification.valid() {
		return &ValidationError{Message: "classification must be 'friend' or 'close_friend'"}
	}
	return nil
}
