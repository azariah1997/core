// Package relationships implements a generic relationship graph. The
// platform never hardcodes what a relationship means - "friend",
// "follower", "partner" are all just product-supplied Type strings. What
// the platform does own is the generic lifecycle (request, accept,
// decline, remove, block) and enforcing it server-side.
package relationships

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusBlocked Status = "blocked"
	StatusEnded   Status = "ended"
)

// Relationship is directional at creation (RequesterID initiated it) even
// though most products treat the resulting active relationship as
// symmetric - the direction matters for permission checks (only the
// target may accept/decline a pending request) and is preserved for
// audit even after that stops mattering.
type Relationship struct {
	ID          string
	AppID       string
	RequesterID string
	TargetID    string
	Type        string // product-defined; the platform never hardcodes this
	Status      Status
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound = errors.New("relationship not found")
	// ErrExists covers pending/active/blocked, but not ended - Request
	// revives an ended relationship back to pending instead (real products
	// need "unfriend, then re-friend" to work), while anything still live
	// goes through Accept/Decline/Remove/Block, not a second Request.
	ErrExists = errors.New("a relationship already exists between these users for this type")
	// ErrInvalidTransition and ErrForbidden are the two ways an action can
	// be refused: the relationship isn't in a state the action applies to
	// (a conflict - the HTTP layer maps this to 409), or the caller isn't
	// the party allowed to perform it (403).
	ErrInvalidTransition = errors.New("relationship is not in a state that allows this action")
	ErrForbidden         = errors.New("not permitted to perform this action on this relationship")
)

type RequestInput struct {
	AppID       string
	RequesterID string
	TargetID    string
	Type        string
	Metadata    map[string]any
}

func (in RequestInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Type) == "":
		return &ValidationError{"type is required"}
	case strings.TrimSpace(in.TargetID) == "":
		return &ValidationError{"targetUserId is required"}
	case in.RequesterID == in.TargetID:
		return &ValidationError{"cannot create a relationship with yourself"}
	}
	return nil
}

type ListFilter struct {
	Type   string // optional
	Status Status // optional
}

// Repository is the storage-agnostic boundary. There is at most one row
// per (AppID, unordered pair, Type) - the uniqueness is symmetric
// regardless of which side is requester/target, matching the existing
// database constraint.
type Repository interface {
	Create(ctx context.Context, in RequestInput, status Status) (Relationship, error)
	Get(ctx context.Context, id string) (Relationship, error)
	// FindBetween looks up the (at most one) row for this pair+type
	// regardless of direction. Returns ErrNotFound if none exists.
	FindBetween(ctx context.Context, appID, userA, userB, relType string) (Relationship, error)
	ListForUser(ctx context.Context, appID, userID string, filter ListFilter) ([]Relationship, error)
	UpdateStatus(ctx context.Context, id string, status Status) (Relationship, error)
	// Revive resets an ended relationship back to pending under a new
	// request (new requester/target/metadata) - real products need
	// "unfriend, then re-friend later" to work, not a permanent lock.
	// Only valid when the existing row's status is ended; callers must
	// check that themselves.
	Revive(ctx context.Context, id string, in RequestInput) (Relationship, error)
}
