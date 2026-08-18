package groups

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create needs no membership check - anyone authenticated may create a
// group, becoming its first manager.
func (s *Service) Create(ctx context.Context, creatorUserID string, in CreateInput) (Group, error) {
	if err := in.Validate(); err != nil {
		return Group{}, err
	}
	return s.repo.Create(ctx, creatorUserID, in)
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Group, error) {
	if _, err := s.requireMember(ctx, id, callerID); err != nil {
		return Group{}, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) ListMine(ctx context.Context, userID string) ([]Group, error) {
	return s.repo.ListForUser(ctx, userID)
}

func (s *Service) Update(ctx context.Context, callerID, id string, in UpdateInput) (Group, error) {
	if err := s.requireManager(ctx, id, callerID); err != nil {
		return Group{}, err
	}
	if err := in.Validate(); err != nil {
		return Group{}, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) ListMembers(ctx context.Context, callerID, groupID string) ([]GroupMember, error) {
	if _, err := s.requireMember(ctx, groupID, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, groupID)
}

func (s *Service) AddMember(ctx context.Context, callerID, groupID string, in AddMemberInput) (GroupMember, error) {
	if err := s.requireManager(ctx, groupID, callerID); err != nil {
		return GroupMember{}, err
	}
	if err := in.Validate(); err != nil {
		return GroupMember{}, err
	}
	return s.repo.AddMember(ctx, groupID, in)
}

// UpdateMember (changing a member's role label or promoting/demoting their
// manager bit) requires the caller to already be a manager - a plain
// member can never grant themselves or anyone else manager status.
func (s *Service) UpdateMember(ctx context.Context, callerID, groupID, targetUserID string, in UpdateMemberInput) (GroupMember, error) {
	if err := s.requireManager(ctx, groupID, callerID); err != nil {
		return GroupMember{}, err
	}
	if err := in.Validate(); err != nil {
		return GroupMember{}, err
	}
	return s.repo.UpdateMember(ctx, groupID, targetUserID, in)
}

// RemoveMember lets a member remove themselves ("leave") without manager
// status; removing anyone else requires being a manager.
func (s *Service) RemoveMember(ctx context.Context, callerID, groupID, targetUserID string) error {
	if callerID == targetUserID {
		if _, err := s.requireMember(ctx, groupID, callerID); err != nil {
			return err
		}
	} else if err := s.requireManager(ctx, groupID, callerID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, groupID, targetUserID)
}

func (s *Service) requireMember(ctx context.Context, groupID, userID string) (GroupMember, error) {
	m, err := s.repo.GetMembership(ctx, groupID, userID)
	if err != nil {
		return GroupMember{}, ErrForbidden
	}
	return m, nil
}

func (s *Service) requireManager(ctx context.Context, groupID, userID string) error {
	m, err := s.requireMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !m.IsManager {
		return ErrManagerRequired
	}
	return nil
}
