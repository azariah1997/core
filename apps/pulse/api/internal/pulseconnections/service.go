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
