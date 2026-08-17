package users

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	if in.Locale == "" {
		in.Locale = "en-GB"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if err := in.Validate(); err != nil {
		return User{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (User, error) {
	if err := in.Validate(); err != nil {
		return User{}, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id string) (User, error) {
	return s.repo.Delete(ctx, id)
}

// IdentityLinker is the one thing EnsureForIdentity needs from identity -
// declared here, satisfied structurally by identity.Service, so this
// package never imports identity and stays independently understandable.
type IdentityLinker interface {
	LinkUser(ctx context.Context, identityID, userID string) error
}

// EnsureForIdentity resolves the User already linked to an identity, or
// provisions one on first login and links it. This is the one place
// Phases 3 and 4 meet - identity itself never creates or links Users.
func (s *Service) EnsureForIdentity(ctx context.Context, linker IdentityLinker, identityID string, existingUserID *string, suggestedDisplayName string) (User, error) {
	if existingUserID != nil {
		return s.Get(ctx, *existingUserID)
	}
	displayName := suggestedDisplayName
	if displayName == "" {
		displayName = "New User"
	}
	user, err := s.Create(ctx, CreateInput{DisplayName: displayName})
	if err != nil {
		return User{}, err
	}
	if err := linker.LinkUser(ctx, identityID, user.ID); err != nil {
		return User{}, err
	}
	return user, nil
}
