package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// RateLimiter is satisfied directly by *platformkit/ratelimit.Limiter -
// the same package Phase 21's report-spam protection introduced, reused
// here for a second, unrelated reason: Track has no Bearer token to key
// a limiter by, so it uses the caller's IP address instead.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

const (
	trackRateLimit       = 300
	trackRateLimitWindow = time.Minute

	debugListDefaultLimit = 50
	debugListMaxLimit     = 200
)

type Service struct {
	repo    Repository
	admin   AdminChecker
	limiter RateLimiter
}

func NewService(repo Repository, admin AdminChecker, limiter RateLimiter) *Service {
	return &Service{repo: repo, admin: admin, limiter: limiter}
}

// Track ingests a batch in one call - real client SDKs (Segment,
// Amplitude, PostHog) all batch client-side before sending, and a
// single event is just a batch of one. clientKey rate-limits by
// whatever the HTTP layer chose (IP address; see http.go) since there's
// no authenticated caller identity to key by - this is this platform's
// one deliberately open, unauthenticated write endpoint. Every event's
// UserID/AnonymousID is exactly as trustworthy as any other analytics
// SDK's client-reported identity: self-declared, not verified against a
// real session, the same characteristic Segment/Amplitude/Mixpanel all
// share - a deliberate, accepted property of analytics tracking, unlike
// every other module's spoof-proof identity resolution.
func (s *Service) Track(ctx context.Context, clientKey string, inputs []TrackInput) error {
	allowed, err := s.limiter.Allow(ctx, "analytics:track:"+clientKey, trackRateLimit, trackRateLimitWindow)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}

	now := time.Now().UTC()
	events := make([]Event, 0, len(inputs))
	for _, in := range inputs {
		if err := in.Validate(); err != nil {
			return err
		}
		occurredAt := in.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = now
		}
		events = append(events, Event{
			ID: uuid.NewString(), EventName: in.EventName, UserID: in.UserID, AnonymousID: in.AnonymousID,
			AppID: in.AppID, SessionID: in.SessionID, OccurredAt: occurredAt, Properties: in.Properties,
			Context: in.Context, IngestedAt: now,
		})
	}
	if len(events) == 0 {
		return nil
	}
	return s.repo.RecordBatch(ctx, events)
}

// ListRecent is a small, platform.admin-only debug/operational view -
// "did my last Track call actually land" - not a real analytics query
// surface. Real analytics questions (funnels, cohorts, aggregation) get
// answered downstream, once the pipeline this phase builds lands
// batches somewhere built for that.
func (s *Service) ListRecent(ctx context.Context, callerID string, limit int) ([]Event, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > debugListMaxLimit {
		limit = debugListDefaultLimit
	}
	return s.repo.ListRecent(ctx, limit)
}

func (s *Service) requireAdmin(ctx context.Context, callerID string) error {
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
