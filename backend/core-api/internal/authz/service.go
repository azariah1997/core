package authz

import "context"

// platformResource is the one singleton object platform.admin's fine-grained
// grant is written against - "does this user have admin on the platform as
// a whole", not tied to any specific resource instance.
var platformResource = Resource{Type: "platform", ID: "core"}

const actionAdmin Action = "admin"

type Service struct {
	roles    RoleRepository
	provider Provider
}

func NewService(roles RoleRepository, provider Provider) *Service {
	return &Service{roles: roles, provider: provider}
}

func (s *Service) HasRole(ctx context.Context, userID string, role Role) (bool, error) {
	roles, err := s.roles.RolesFor(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

// AssignRole is the one place a role is granted - for platform.admin it
// also grants the equivalent fine-grained relationship in Provider, so
// Can() sees it too. Postgres is written first: if the OpenFGA write then
// fails, the role still exists and AssignRole can simply be retried: it's
// naturally idempotent, so partial failure here self-heals rather than
// leaving two systems permanently disagreeing.
func (s *Service) AssignRole(ctx context.Context, userID string, role Role) error {
	if err := s.roles.AssignRole(ctx, userID, role); err != nil {
		return err
	}
	if role == RolePlatformAdmin {
		return s.provider.Grant(ctx, userID, actionAdmin, platformResource)
	}
	return nil
}

func (s *Service) RevokeRole(ctx context.Context, userID string, role Role) error {
	if err := s.roles.RevokeRole(ctx, userID, role); err != nil {
		return err
	}
	if role == RolePlatformAdmin {
		return s.provider.Revoke(ctx, userID, actionAdmin, platformResource)
	}
	return nil
}

// Can is the single entry point every domain module must use instead of
// implementing permission logic independently.
func (s *Service) Can(ctx context.Context, subjectUserID string, action Action, resource Resource) (bool, error) {
	return s.provider.Can(ctx, subjectUserID, action, resource)
}

// IsPlatformAdmin is a convenience for the extremely common "does this
// caller have blanket admin access" check, backed by the same fine-grained
// grant AssignRole writes - so it stays consistent with Can() rather than
// being a separate, potentially-drifting code path.
func (s *Service) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return s.Can(ctx, userID, actionAdmin, platformResource)
}
