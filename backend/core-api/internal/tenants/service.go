package tenants

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create needs no membership check - anyone authenticated may create a
// tenant, becoming its owner.
func (s *Service) Create(ctx context.Context, ownerUserID string, in CreateInput) (Tenant, error) {
	if err := in.Validate(); err != nil {
		return Tenant{}, err
	}
	return s.repo.Create(ctx, ownerUserID, in)
}

func (s *Service) Get(ctx context.Context, callerUserID, tenantID string) (Tenant, error) {
	if _, err := s.requireMember(ctx, tenantID, callerUserID); err != nil {
		return Tenant{}, err
	}
	return s.repo.Get(ctx, tenantID)
}

func (s *Service) ListMine(ctx context.Context, userID string) ([]Tenant, error) {
	return s.repo.ListForUser(ctx, userID)
}

func (s *Service) Update(ctx context.Context, callerUserID, tenantID string, in UpdateInput) (Tenant, error) {
	if err := s.requireManager(ctx, tenantID, callerUserID); err != nil {
		return Tenant{}, err
	}
	if err := in.Validate(); err != nil {
		return Tenant{}, err
	}
	return s.repo.Update(ctx, tenantID, in)
}

func (s *Service) ListMembers(ctx context.Context, callerUserID, tenantID string) ([]Membership, error) {
	if _, err := s.requireMember(ctx, tenantID, callerUserID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, tenantID)
}

func (s *Service) AddMember(ctx context.Context, callerUserID, tenantID string, in AddMemberInput) (Membership, error) {
	if err := s.requireManager(ctx, tenantID, callerUserID); err != nil {
		return Membership{}, err
	}
	if err := in.Validate(); err != nil {
		return Membership{}, err
	}
	return s.repo.AddMember(ctx, tenantID, in)
}

// RemoveMember lets a member remove themselves ("leave") without manager
// status; removing anyone else requires owner/admin.
func (s *Service) RemoveMember(ctx context.Context, callerUserID, tenantID, targetUserID string) error {
	if callerUserID == targetUserID {
		if _, err := s.requireMember(ctx, tenantID, callerUserID); err != nil {
			return err
		}
	} else if err := s.requireManager(ctx, tenantID, callerUserID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, tenantID, targetUserID)
}

func (s *Service) requireMember(ctx context.Context, tenantID, userID string) (Membership, error) {
	m, err := s.repo.GetMembership(ctx, tenantID, userID)
	if err != nil {
		return Membership{}, ErrForbidden
	}
	return m, nil
}

func (s *Service) requireManager(ctx context.Context, tenantID, userID string) error {
	m, err := s.requireMember(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if !m.Role.canManage() {
		return ErrManagerRequired
	}
	return nil
}
