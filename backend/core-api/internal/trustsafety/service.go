package trustsafety

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ModeratorChecker is satisfied directly by *authz.Service (Phase 21
// adds IsModerator alongside its existing IsPlatformAdmin) - no adapter
// needed, the same pattern as every other module's AdminChecker.
type ModeratorChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
	IsModerator(ctx context.Context, userID string) (bool, error)
}

// RateLimiter is satisfied directly by *platformkit/ratelimit.Limiter.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

const (
	// reportRateLimit is this phase's one concrete "rate limiting" and
	// "spam protection" integration - deliberately scoped to the one
	// action in this module a bad actor would most want to spam
	// (filing false reports to trigger moderation review), rather than
	// retrofitting a limiter onto every write in this package.
	reportRateLimit       = 5
	reportRateLimitWindow = time.Hour

	// criticalSignalAutoEscalates is the deliberately simple abuse-
	// signal escalation rule this phase implements: a single critical
	// signal opens (or attaches to) a case immediately. Anything more
	// elaborate (aggregating lower-severity signals over a window into
	// an escalation) is real abuse-detection engine territory, out of
	// this phase's scope.
	autoEscalateSeverity = SeverityCritical
)

type Service struct {
	repo      Repository
	moderator ModeratorChecker
	limiter   RateLimiter
}

func NewService(repo Repository, moderator ModeratorChecker, limiter RateLimiter) *Service {
	return &Service{repo: repo, moderator: moderator, limiter: limiter}
}

func (s *Service) Mute(ctx context.Context, callerID, mutedUserID string) (Mute, error) {
	if callerID == mutedUserID {
		return Mute{}, &ValidationError{"cannot mute yourself"}
	}
	m := Mute{ID: uuid.NewString(), MuterUserID: callerID, MutedUserID: mutedUserID, CreatedAt: time.Now().UTC()}
	if err := s.repo.Mute(ctx, m); err != nil {
		return Mute{}, err
	}
	return m, nil
}

func (s *Service) Unmute(ctx context.Context, callerID, mutedUserID string) error {
	return s.repo.Unmute(ctx, callerID, mutedUserID)
}

func (s *Service) ListMutes(ctx context.Context, callerID string) ([]Mute, error) {
	return s.repo.ListMutes(ctx, callerID)
}

// CreateReport is rate-limited per reporter - the concrete spam
// protection this phase implements. A successful report always lands
// on an open case, creating one if this resource has none open yet
// (OpenOrAttachCase), so a moderator reviews one case with N reports
// attached instead of N separate cases for the same resource.
func (s *Service) CreateReport(ctx context.Context, callerID string, in ReportInput) (Report, error) {
	if err := in.Validate(); err != nil {
		return Report{}, err
	}
	allowed, err := s.limiter.Allow(ctx, "trustsafety:report:"+callerID, reportRateLimit, reportRateLimitWindow)
	if err != nil {
		return Report{}, err
	}
	if !allowed {
		return Report{}, ErrRateLimited
	}
	c, err := s.repo.OpenOrAttachCase(ctx, in.ResourceType, in.ResourceID)
	if err != nil {
		return Report{}, err
	}
	r := Report{
		ID: uuid.NewString(), ReporterUserID: callerID, ResourceType: in.ResourceType, ResourceID: in.ResourceID,
		Reason: in.Reason, Details: in.Details, CaseID: c.ID, Status: ReportStatusOpen, CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateReport(ctx, r); err != nil {
		return Report{}, err
	}
	return r, nil
}

func (s *Service) GetReport(ctx context.Context, callerID, id string) (Report, error) {
	r, err := s.repo.GetReport(ctx, id)
	if err != nil {
		return Report{}, err
	}
	if r.ReporterUserID != callerID {
		if err := s.requireModerator(ctx, callerID); err != nil {
			return Report{}, err
		}
	}
	return r, nil
}

func (s *Service) ListCaseReports(ctx context.Context, callerID, caseID string) ([]Report, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListReports(ctx, caseID)
}

func (s *Service) ListCases(ctx context.Context, callerID string, status CaseStatus) ([]ModerationCase, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListCases(ctx, status)
}

func (s *Service) GetCase(ctx context.Context, callerID, id string) (ModerationCase, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return ModerationCase{}, err
	}
	return s.repo.GetCase(ctx, id)
}

func (s *Service) AssignCase(ctx context.Context, callerID, id, assigneeUserID string) (ModerationCase, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return ModerationCase{}, err
	}
	return s.repo.AssignCase(ctx, id, assigneeUserID)
}

func (s *Service) ResolveCase(ctx context.Context, callerID, id string, in ResolveCaseInput) (ModerationCase, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return ModerationCase{}, err
	}
	if err := in.Validate(); err != nil {
		return ModerationCase{}, err
	}
	return s.repo.ResolveCase(ctx, id, in)
}

func (s *Service) Suspend(ctx context.Context, callerID string, in SuspendInput) (Suspension, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return Suspension{}, err
	}
	if err := in.Validate(); err != nil {
		return Suspension{}, err
	}
	now := time.Now().UTC()
	sus := Suspension{
		ID: uuid.NewString(), UserID: in.UserID, Reason: in.Reason, CaseID: in.CaseID, IssuedBy: callerID,
		StartsAt: now, EndsAt: now.Add(in.Duration), CreatedAt: now,
	}
	if err := s.repo.CreateSuspension(ctx, sus); err != nil {
		return Suspension{}, err
	}
	return sus, nil
}

func (s *Service) ListSuspensions(ctx context.Context, callerID, targetUserID string) ([]Suspension, error) {
	if err := s.requireOwnerOrModerator(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	return s.repo.ListSuspensions(ctx, targetUserID)
}

func (s *Service) LiftSuspension(ctx context.Context, callerID, id string) (Suspension, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return Suspension{}, err
	}
	return s.repo.LiftSuspension(ctx, id, callerID)
}

func (s *Service) Ban(ctx context.Context, callerID string, in BanInput) (Ban, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return Ban{}, err
	}
	if err := in.Validate(); err != nil {
		return Ban{}, err
	}
	b := Ban{ID: uuid.NewString(), UserID: in.UserID, Reason: in.Reason, CaseID: in.CaseID, IssuedBy: callerID, CreatedAt: time.Now().UTC()}
	if err := s.repo.CreateBan(ctx, b); err != nil {
		return Ban{}, err
	}
	return b, nil
}

func (s *Service) ListBans(ctx context.Context, callerID, targetUserID string) ([]Ban, error) {
	if err := s.requireOwnerOrModerator(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	return s.repo.ListBans(ctx, targetUserID)
}

func (s *Service) LiftBan(ctx context.Context, callerID, id string) (Ban, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return Ban{}, err
	}
	return s.repo.LiftBan(ctx, id, callerID)
}

// CreateAppeal verifies the caller actually owns the suspension or ban
// they're appealing - a stranger can't file an appeal against someone
// else's restriction, even though the resulting appeal review is
// moderator/admin-only either way.
func (s *Service) CreateAppeal(ctx context.Context, callerID string, in AppealInput) (Appeal, error) {
	if err := in.Validate(); err != nil {
		return Appeal{}, err
	}
	var ownerID string
	switch in.TargetType {
	case AppealTargetSuspension:
		sus, err := s.repo.GetSuspension(ctx, in.TargetID)
		if err != nil {
			return Appeal{}, err
		}
		ownerID = sus.UserID
	case AppealTargetBan:
		b, err := s.repo.GetBan(ctx, in.TargetID)
		if err != nil {
			return Appeal{}, err
		}
		ownerID = b.UserID
	}
	if ownerID != callerID {
		return Appeal{}, ErrForbidden
	}
	a := Appeal{
		ID: uuid.NewString(), UserID: callerID, TargetType: in.TargetType, TargetID: in.TargetID,
		Reason: in.Reason, Status: AppealStatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateAppeal(ctx, a); err != nil {
		return Appeal{}, err
	}
	return a, nil
}

func (s *Service) ListAppeals(ctx context.Context, callerID, targetUserID string) ([]Appeal, error) {
	if err := s.requireOwnerOrModerator(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	return s.repo.ListAppeals(ctx, targetUserID)
}

// ReviewAppeal is the one place approving an appeal has a real side
// effect: it lifts the appealed Suspension or Ban in the same call,
// rather than leaving a moderator to separately remember to do so.
func (s *Service) ReviewAppeal(ctx context.Context, callerID, id string, approve bool) (Appeal, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return Appeal{}, err
	}
	a, err := s.repo.GetAppeal(ctx, id)
	if err != nil {
		return Appeal{}, err
	}
	if a.Status != AppealStatusPending {
		return Appeal{}, ErrAlreadyReviewed
	}
	status := AppealStatusDenied
	if approve {
		status = AppealStatusApproved
		switch a.TargetType {
		case AppealTargetSuspension:
			if _, err := s.repo.LiftSuspension(ctx, a.TargetID, callerID); err != nil {
				return Appeal{}, err
			}
		case AppealTargetBan:
			if _, err := s.repo.LiftBan(ctx, a.TargetID, callerID); err != nil {
				return Appeal{}, err
			}
		}
	}
	return s.repo.ReviewAppeal(ctx, id, status, callerID)
}

// RecordSignal is open to any authenticated caller or trusted in-
// process Go caller (mirroring audit.Record's own reasoning) - a
// critical signal immediately opens or attaches to a case, the one
// deliberately simple auto-escalation rule this phase implements.
func (s *Service) RecordSignal(ctx context.Context, in SignalInput) (AbuseSignal, error) {
	if err := in.Validate(); err != nil {
		return AbuseSignal{}, err
	}
	sig := AbuseSignal{
		ID: uuid.NewString(), ResourceType: in.ResourceType, ResourceID: in.ResourceID,
		SignalType: in.SignalType, Severity: string(in.Severity), Metadata: in.Metadata, RecordedAt: time.Now().UTC(),
	}
	if err := s.repo.RecordSignal(ctx, sig); err != nil {
		return AbuseSignal{}, err
	}
	if in.Severity == autoEscalateSeverity {
		if _, err := s.repo.OpenOrAttachCase(ctx, in.ResourceType, in.ResourceID); err != nil {
			return AbuseSignal{}, err
		}
	}
	return sig, nil
}

func (s *Service) ListSignals(ctx context.Context, callerID, resourceType, resourceID string) ([]AbuseSignal, error) {
	if err := s.requireModerator(ctx, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListSignals(ctx, resourceType, resourceID)
}

// IsRestricted is the platform-wide enforcement hook - internal/api's
// requireActive middleware calls this once, right after resolving the
// caller's platform user, so Suspension/Ban actually restrict access
// instead of being inert rows nobody checks. See package api's doc
// comment on requireActive for which routes are (and deliberately are
// not) gated by it.
func (s *Service) IsRestricted(ctx context.Context, userID string) (bool, string, error) {
	bans, err := s.repo.ListBans(ctx, userID)
	if err != nil {
		return false, "", err
	}
	for _, b := range bans {
		if b.IsActive() {
			return true, "banned: " + b.Reason, nil
		}
	}
	suspensions, err := s.repo.ListSuspensions(ctx, userID)
	if err != nil {
		return false, "", err
	}
	now := time.Now().UTC()
	for _, sus := range suspensions {
		if sus.IsActive(now) {
			return true, "suspended: " + sus.Reason, nil
		}
	}
	return false, "", nil
}

func (s *Service) requireModerator(ctx context.Context, callerID string) error {
	isAdmin, err := s.moderator.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if isAdmin {
		return nil
	}
	isModerator, err := s.moderator.IsModerator(ctx, callerID)
	if err != nil {
		return err
	}
	if !isModerator {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireOwnerOrModerator(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	return s.requireModerator(ctx, callerID)
}

var ErrRateLimited = errors.New("too many reports - try again later")
