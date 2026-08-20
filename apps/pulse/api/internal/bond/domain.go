// Package bond implements Pulse's one special romantic connection
// (product spec §7): request, mutual acceptance (never automatic), and
// the one-active-Bond-per-user policy. Like pulse-connections, the
// underlying connection is a real Core relationship (type "pulse_bond",
// distinct from pulse-connections' "pulse_friend" so the two are never
// confused in Core's shared graph) - but the one-active-bond invariant
// itself is Pulse product policy the spec explicitly says should never
// be embedded into Core, so it's enforced entirely in Pulse's own
// storage (see Repository's doc comment for the real mechanism).
package bond

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RelationshipType is the Core relationships.Type every Bond request
// creates. FriendRelationshipType must already exist and be active
// between two users before a Bond can be requested (product spec §11:
// "Existing connection -> Request Bond -> Explicit confirmation ->
// Active Bond") - checked via CoreRelationships.ListMine, not
// duplicated as a second connection concept.
const (
	RelationshipType       = "pulse_bond"
	FriendRelationshipType = "pulse_friend"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusEnded   Status = "ended"
)

// RelationshipRef mirrors pulse-connections' own type of the same name -
// duplicated deliberately rather than imported, matching this
// codebase's consumer-defined-interface convention (every Core module
// defines its own local AdminChecker even when the same authz.Service
// satisfies all of them; see AI_CONTEXT.md).
type RelationshipRef struct {
	ID          string
	RequesterID string
	TargetID    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Bond is Pulse's own record - Core's relationship row is the
// connection; this is the bond-specific state layered on top (who
// requested it, whether it's the holder of the one-active-bond slot).
type Bond struct {
	ID             string
	RelationshipID string
	UserAID        string // requester
	UserBID        string // target - the only one who may accept/decline
	Status         Status
	RequestedAt    time.Time
	AcceptedAt     *time.Time
	EndedAt        *time.Time
	UpdatedAt      time.Time
}

func (b Bond) otherUser(callerID string) string {
	if b.UserAID == callerID {
		return b.UserBID
	}
	return b.UserAID
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound = errors.New("bond not found")
	// ErrForbidden covers both "not a participant" and, for
	// Accept/Decline specifically, "participant but not the target" -
	// mirroring Core's own relationships.ErrForbidden semantics
	// (only the target may accept/decline a pending request).
	ErrForbidden = errors.New("not permitted to perform this action on this bond")
	// ErrNoConnection enforces product spec §11's required order:
	// Bond can only be requested over an existing active connection.
	ErrNoConnection = errors.New("an active connection must exist before requesting a bond")
	// ErrAlreadyBonded is the real, race-free answer to "one of these
	// users already holds an active bond" - see Repository's doc
	// comment for the actual enforcement mechanism this maps from.
	ErrAlreadyBonded = errors.New("one of these users already has an active bond")
)

// CoreRelationships is the subset of Core's real relationships API this
// module needs, scoped to a single already-authenticated caller -
// http.go builds one per request from pulseauth's resolved client. Same
// shape as pulse-connections' own interface of the same name,
// duplicated rather than shared for the same consumer-defined-interface
// reason as RelationshipRef above.
type CoreRelationships interface {
	Request(ctx context.Context, targetUserID, relType string) (RelationshipRef, error)
	Accept(ctx context.Context, relationshipID string) (RelationshipRef, error)
	Decline(ctx context.Context, relationshipID string) (RelationshipRef, error)
	Remove(ctx context.Context, relationshipID string) error
	ListMine(ctx context.Context, relType string) ([]RelationshipRef, error)
}

// Repository is Pulse's own bond storage - and the real place the
// one-active-bond-per-user invariant is enforced. The production
// (postgres) implementation does this with a dedicated
// bond_active_holders table keyed by user_id alone (PRIMARY KEY, one
// row per user, full stop) - Accept inserts both participants into it
// in the same transaction as flipping the bond to active, so a second
// concurrent Accept trying to bond either participant to someone else
// hits a real unique-constraint violation and rolls back atomically.
// This is deliberately not "check then write" application logic, which
// cannot be race-free under concurrent Accept calls (see
// apps/pulse/docs/ARCHITECTURE_AUDIT.md's Risk #2).
type Repository interface {
	Create(ctx context.Context, b Bond) (Bond, error)
	Get(ctx context.Context, id string) (Bond, error)
	// Accept transitions pending -> active and enforces the
	// one-active-bond invariant atomically. Returns ErrAlreadyBonded,
	// never a silent double-bond, if either participant already holds
	// an active bond with anyone (including each other, twice).
	Accept(ctx context.Context, id string) (Bond, error)
	End(ctx context.Context, id string) (Bond, error)
	// ActiveForUser returns the caller's current active bond. Returns
	// ErrNotFound if they don't currently hold one.
	ActiveForUser(ctx context.Context, userID string) (Bond, error)
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
