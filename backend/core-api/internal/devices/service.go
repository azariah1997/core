package devices

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, userID string, in RegisterInput) (Device, error) {
	if in.Locale == "" {
		in.Locale = "en-GB"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if err := in.Validate(); err != nil {
		return Device{}, err
	}
	return s.repo.Register(ctx, userID, in)
}

func (s *Service) List(ctx context.Context, userID string) ([]Device, error) {
	return s.repo.List(ctx, userID)
}

func (s *Service) Revoke(ctx context.Context, userID, deviceID string) error {
	return s.repo.Revoke(ctx, userID, deviceID)
}
