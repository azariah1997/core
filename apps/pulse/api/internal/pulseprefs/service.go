package pulseprefs

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context, userID string) (Preferences, error) {
	return s.repo.Get(ctx, userID)
}

func (s *Service) Set(ctx context.Context, userID string, in SetPreferencesInput) (Preferences, error) {
	if err := in.Validate(); err != nil {
		return Preferences{}, err
	}
	p := Preferences{UserID: userID, NotificationDetail: in.NotificationDetail, HapticIntensity: in.HapticIntensity, UpdatedAt: time.Now().UTC()}
	return s.repo.Set(ctx, p)
}

func (s *Service) GetQuietHours(ctx context.Context, core CoreQuietHours) (QuietHours, error) {
	return core.Get(ctx)
}

func (s *Service) SetQuietHours(ctx context.Context, core CoreQuietHours, in SetQuietHoursInput) (QuietHours, error) {
	if err := in.Validate(); err != nil {
		return QuietHours{}, err
	}
	return core.Set(ctx, in)
}

func (s *Service) Mute(ctx context.Context, core CoreMutes, mutedUserID string) (Mute, error) {
	return core.Mute(ctx, mutedUserID)
}

func (s *Service) ListMutes(ctx context.Context, core CoreMutes) ([]Mute, error) {
	return core.List(ctx)
}

func (s *Service) Unmute(ctx context.Context, core CoreMutes, mutedUserID string) error {
	return core.Unmute(ctx, mutedUserID)
}
