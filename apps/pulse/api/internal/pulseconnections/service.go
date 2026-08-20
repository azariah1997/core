package pulseconnections

import (
	"context"
)

type Service struct {
	classifications ClassificationRepository
}

func NewService(classifications ClassificationRepository) *Service {
	return &Service{classifications: classifications}
}

func mergeConnection(callerID string, rel RelationshipRef, classification Classification) Connection {
	otherID, direction := rel.TargetID, "outgoing"
	if rel.RequesterID != callerID {
		otherID, direction = rel.RequesterID, "incoming"
	}
	return Connection{
		RelationshipID: rel.ID, OwnerUserID: callerID, OtherUserID: otherID,
		Status: rel.Status, Direction: direction, Classification: classification,
		CreatedAt: rel.CreatedAt, UpdatedAt: rel.UpdatedAt,
	}
}

// RequestConnection asks Core to create (or revive) a "pulse_friend"
// relationship - the connection lifecycle itself is entirely Core's,
// this just calls it with Pulse's own relationship type.
func (s *Service) RequestConnection(ctx context.Context, core CoreRelationships, callerID string, in RequestInput) (Connection, error) {
	if err := in.Validate(); err != nil {
		return Connection{}, err
	}
	rel, err := core.Request(ctx, in.TargetUserID, RelationshipType)
	if err != nil {
		return Connection{}, err
	}
	return mergeConnection(callerID, rel, ClassificationFriend), nil
}

func (s *Service) Accept(ctx context.Context, core CoreRelationships, callerID, relationshipID string) (Connection, error) {
	rel, err := core.Accept(ctx, relationshipID)
	if err != nil {
		return Connection{}, err
	}
	classification, err := s.classifications.Get(ctx, relationshipID, callerID)
	if err != nil {
		return Connection{}, err
	}
	return mergeConnection(callerID, rel, classification), nil
}

func (s *Service) Decline(ctx context.Context, core CoreRelationships, callerID, relationshipID string) (Connection, error) {
	rel, err := core.Decline(ctx, relationshipID)
	if err != nil {
		return Connection{}, err
	}
	return mergeConnection(callerID, rel, ClassificationFriend), nil
}

func (s *Service) Remove(ctx context.Context, core CoreRelationships, relationshipID string) error {
	return core.Remove(ctx, relationshipID)
}

// ListMine returns every Pulse connection (any status) for the caller,
// merged with whatever classification they've set - defaulting to
// ClassificationFriend for any relationship never explicitly classified.
func (s *Service) ListMine(ctx context.Context, core CoreRelationships, callerID string) ([]Connection, error) {
	rels, err := core.ListMine(ctx, RelationshipType)
	if err != nil {
		return nil, err
	}
	classifications, err := s.classifications.ListForOwner(ctx, callerID)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rels))
	for _, rel := range rels {
		c := classifications[rel.ID]
		if c == "" {
			c = ClassificationFriend
		}
		out = append(out, mergeConnection(callerID, rel, c))
	}
	return out, nil
}

// SetClassification is Pulse's own mutation - no Core call, since
// Friend vs Close Friend is Pulse-specific data Core never sees.
func (s *Service) SetClassification(ctx context.Context, callerID, relationshipID string, in SetClassificationInput) (Classification, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	if err := s.classifications.Set(ctx, relationshipID, callerID, in.Classification); err != nil {
		return "", err
	}
	return in.Classification, nil
}

// checkConnected mirrors pulse-interactions' own helper of the same
// name and meaning: an active Friend or Bond connection, checked from
// the caller's own real relationships (never a stranger, never a
// blocked relationship, in either direction).
func checkConnected(ctx context.Context, core CoreRelationships, otherUserID string) error {
	for _, relType := range []string{RelationshipType, bondRelationshipType} {
		rels, err := core.ListMine(ctx, relType)
		if err != nil {
			return err
		}
		for _, r := range rels {
			if r.RequesterID != otherUserID && r.TargetID != otherUserID {
				continue
			}
			if r.Status == "active" {
				return nil
			}
		}
	}
	return ErrNotConnected
}

// CreateCircle creates a real Core Group scoped to Pulse's AppID - Core's
// own Create already writes a manager membership for the creator in the
// same transaction (backend/core-api/internal/groups' own guarantee:
// "a group can never transiently exist with no manager"), so the caller
// is already a member of their own new Circle with no separate call.
func (s *Service) CreateCircle(ctx context.Context, groups CoreGroups, in CreateCircleInput) (Circle, error) {
	if err := in.Validate(); err != nil {
		return Circle{}, err
	}
	return groups.Create(ctx, in.Name)
}

func (s *Service) ListMyCircles(ctx context.Context, groups CoreGroups) ([]Circle, error) {
	return groups.ListMine(ctx)
}

func (s *Service) ListCircleMembers(ctx context.Context, groups CoreGroups, circleID string) ([]CircleMember, error) {
	return groups.ListMembers(ctx, circleID)
}

// AddCircleMember requires the caller and the target to already be a
// real, active Pulse connection (product spec §10-11: Circles exist to
// control interaction audience/permissions among people you're already
// connected to, never a way to reach a stranger) - Core's own
// authorization (the caller must be a Circle manager) is enforced
// separately, server-side, by groups.AddMember itself.
func (s *Service) AddCircleMember(ctx context.Context, groups CoreGroups, core CoreRelationships, circleID, targetUserID string) (CircleMember, error) {
	if err := checkConnected(ctx, core, targetUserID); err != nil {
		return CircleMember{}, err
	}
	return groups.AddMember(ctx, circleID, targetUserID)
}

func (s *Service) RemoveCircleMember(ctx context.Context, groups CoreGroups, circleID, targetUserID string) error {
	return groups.RemoveMember(ctx, circleID, targetUserID)
}
