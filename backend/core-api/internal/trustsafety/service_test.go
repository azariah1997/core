package trustsafety_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
	"github.com/example/core-platform/backend/core-api/internal/trustsafety/memory"
)

type fakeModerator struct {
	admins     map[string]bool
	moderators map[string]bool
}

func (f fakeModerator) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return f.admins[userID], nil
}
func (f fakeModerator) IsModerator(ctx context.Context, userID string) (bool, error) {
	return f.moderators[userID], nil
}

type fakeLimiter struct {
	allow bool
}

func (f fakeLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return f.allow, nil
}

func newService(mod fakeModerator, allow bool) *trustsafety.Service {
	return trustsafety.NewService(memory.New(), mod, fakeLimiter{allow: allow})
}

func TestMuteCannotTargetSelf(t *testing.T) {
	svc := newService(fakeModerator{}, true)
	_, err := svc.Mute(context.Background(), "u1", "u1")
	var verr *trustsafety.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestMuteListUnmute(t *testing.T) {
	svc := newService(fakeModerator{}, true)
	ctx := context.Background()
	if _, err := svc.Mute(ctx, "u1", "u2"); err != nil {
		t.Fatalf("mute: %v", err)
	}
	list, err := svc.ListMutes(ctx, "u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 mute, got %v err=%v", list, err)
	}
	if err := svc.Unmute(ctx, "u1", "u2"); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	list, _ = svc.ListMutes(ctx, "u1")
	if len(list) != 0 {
		t.Fatalf("expected 0 mutes after unmute, got %d", len(list))
	}
}

func TestCreateReportIsRateLimited(t *testing.T) {
	svc := newService(fakeModerator{}, false)
	_, err := svc.CreateReport(context.Background(), "u1", trustsafety.ReportInput{ResourceType: "message", ResourceID: "m1", Reason: "spam"})
	if !errors.Is(err, trustsafety.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestMultipleReportsOnSameResourceAttachToOneCase(t *testing.T) {
	svc := newService(fakeModerator{admins: map[string]bool{"admin": true}}, true)
	ctx := context.Background()
	r1, err := svc.CreateReport(ctx, "u1", trustsafety.ReportInput{ResourceType: "message", ResourceID: "m1", Reason: "spam"})
	if err != nil {
		t.Fatalf("report 1: %v", err)
	}
	r2, err := svc.CreateReport(ctx, "u2", trustsafety.ReportInput{ResourceType: "message", ResourceID: "m1", Reason: "abuse"})
	if err != nil {
		t.Fatalf("report 2: %v", err)
	}
	if r1.CaseID != r2.CaseID {
		t.Fatalf("expected both reports to attach to the same case, got %s and %s", r1.CaseID, r2.CaseID)
	}
	c, err := svc.GetCase(ctx, "admin", r1.CaseID)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	if c.ReportCount != 2 {
		t.Fatalf("expected ReportCount 2, got %d", c.ReportCount)
	}
	reports, err := svc.ListCaseReports(ctx, "admin", r1.CaseID)
	if err != nil || len(reports) != 2 {
		t.Fatalf("expected 2 reports on the case, got %v err=%v", reports, err)
	}
}

func TestListCasesRequiresModerator(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	if _, err := svc.CreateReport(ctx, "u1", trustsafety.ReportInput{ResourceType: "user", ResourceID: "u2", Reason: "harassment"}); err != nil {
		t.Fatalf("report: %v", err)
	}
	if _, err := svc.ListCases(ctx, "u1", ""); !errors.Is(err, trustsafety.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-moderator, got %v", err)
	}
	list, err := svc.ListCases(ctx, "mod", "")
	if err != nil {
		t.Fatalf("list as moderator: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 case, got %d", len(list))
	}
}

func TestResolveCaseRequiresModerator(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	r, err := svc.CreateReport(ctx, "u1", trustsafety.ReportInput{ResourceType: "user", ResourceID: "u2", Reason: "harassment"})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if _, err := svc.ResolveCase(ctx, "u1", r.CaseID, trustsafety.ResolveCaseInput{Status: trustsafety.CaseStatusResolved}); !errors.Is(err, trustsafety.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-moderator, got %v", err)
	}
	c, err := svc.ResolveCase(ctx, "mod", r.CaseID, trustsafety.ResolveCaseInput{Status: trustsafety.CaseStatusResolved, Resolution: "warned"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if c.Status != trustsafety.CaseStatusResolved || c.ResolvedAt == nil {
		t.Fatalf("expected resolved case, got %+v", c)
	}
}

func TestSuspendRequiresModeratorAndComputesActive(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	if _, err := svc.Suspend(ctx, "u1", trustsafety.SuspendInput{UserID: "target", Reason: "spam", Duration: time.Hour}); !errors.Is(err, trustsafety.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-moderator, got %v", err)
	}
	sus, err := svc.Suspend(ctx, "mod", trustsafety.SuspendInput{UserID: "target", Reason: "spam", Duration: time.Hour})
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if !sus.IsActive(time.Now().UTC()) {
		t.Fatal("expected a freshly-created suspension to be active")
	}
	restricted, reason, err := svc.IsRestricted(ctx, "target")
	if err != nil {
		t.Fatalf("is restricted: %v", err)
	}
	if !restricted || reason == "" {
		t.Fatalf("expected the target to be restricted, got restricted=%v reason=%q", restricted, reason)
	}
}

func TestSuspensionExpiresNaturally(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	sus, err := svc.Suspend(ctx, "mod", trustsafety.SuspendInput{UserID: "target", Reason: "spam", Duration: time.Hour})
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if sus.IsActive(time.Now().UTC().Add(2 * time.Hour)) {
		t.Fatal("expected the suspension to no longer be active after its EndsAt has passed")
	}
}

func TestBanIsActiveUntilLifted(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	b, err := svc.Ban(ctx, "mod", trustsafety.BanInput{UserID: "target", Reason: "abuse"})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	if !b.IsActive() {
		t.Fatal("expected a freshly-created ban to be active")
	}
	restricted, _, err := svc.IsRestricted(ctx, "target")
	if err != nil || !restricted {
		t.Fatalf("expected target to be restricted, got %v err=%v", restricted, err)
	}
	lifted, err := svc.LiftBan(ctx, "mod", b.ID)
	if err != nil {
		t.Fatalf("lift: %v", err)
	}
	if lifted.IsActive() {
		t.Fatal("expected the ban to no longer be active after lifting")
	}
	restricted, _, err = svc.IsRestricted(ctx, "target")
	if err != nil || restricted {
		t.Fatalf("expected target to no longer be restricted after the ban was lifted, got %v err=%v", restricted, err)
	}
}

func TestAppealMustBeFiledByTheRestrictedUser(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	b, err := svc.Ban(ctx, "mod", trustsafety.BanInput{UserID: "target", Reason: "abuse"})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, err := svc.CreateAppeal(ctx, "someone-else", trustsafety.AppealInput{TargetType: trustsafety.AppealTargetBan, TargetID: b.ID, Reason: "not me"}); !errors.Is(err, trustsafety.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger filing an appeal, got %v", err)
	}
	a, err := svc.CreateAppeal(ctx, "target", trustsafety.AppealInput{TargetType: trustsafety.AppealTargetBan, TargetID: b.ID, Reason: "it was a mistake"})
	if err != nil {
		t.Fatalf("expected the ban's own target to file an appeal, got %v", err)
	}
	if a.Status != trustsafety.AppealStatusPending {
		t.Fatalf("expected a pending appeal, got %v", a.Status)
	}
}

func TestApprovingAnAppealLiftsTheBan(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	b, err := svc.Ban(ctx, "mod", trustsafety.BanInput{UserID: "target", Reason: "abuse"})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	a, err := svc.CreateAppeal(ctx, "target", trustsafety.AppealInput{TargetType: trustsafety.AppealTargetBan, TargetID: b.ID, Reason: "mistake"})
	if err != nil {
		t.Fatalf("appeal: %v", err)
	}
	reviewed, err := svc.ReviewAppeal(ctx, "mod", a.ID, true)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if reviewed.Status != trustsafety.AppealStatusApproved {
		t.Fatalf("expected approved status, got %v", reviewed.Status)
	}
	restricted, _, err := svc.IsRestricted(ctx, "target")
	if err != nil || restricted {
		t.Fatalf("expected the ban to be lifted as a side effect of approving the appeal, got restricted=%v err=%v", restricted, err)
	}
}

func TestDenyingAnAppealLeavesTheBanActive(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	b, err := svc.Ban(ctx, "mod", trustsafety.BanInput{UserID: "target", Reason: "abuse"})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	a, err := svc.CreateAppeal(ctx, "target", trustsafety.AppealInput{TargetType: trustsafety.AppealTargetBan, TargetID: b.ID, Reason: "mistake"})
	if err != nil {
		t.Fatalf("appeal: %v", err)
	}
	if _, err := svc.ReviewAppeal(ctx, "mod", a.ID, false); err != nil {
		t.Fatalf("review: %v", err)
	}
	restricted, _, err := svc.IsRestricted(ctx, "target")
	if err != nil || !restricted {
		t.Fatalf("expected the ban to remain active after a denied appeal, got restricted=%v err=%v", restricted, err)
	}
}

func TestAppealCannotBeReviewedTwice(t *testing.T) {
	svc := newService(fakeModerator{moderators: map[string]bool{"mod": true}}, true)
	ctx := context.Background()
	b, err := svc.Ban(ctx, "mod", trustsafety.BanInput{UserID: "target", Reason: "abuse"})
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	a, err := svc.CreateAppeal(ctx, "target", trustsafety.AppealInput{TargetType: trustsafety.AppealTargetBan, TargetID: b.ID, Reason: "mistake"})
	if err != nil {
		t.Fatalf("appeal: %v", err)
	}
	if _, err := svc.ReviewAppeal(ctx, "mod", a.ID, true); err != nil {
		t.Fatalf("first review: %v", err)
	}
	if _, err := svc.ReviewAppeal(ctx, "mod", a.ID, false); !errors.Is(err, trustsafety.ErrAlreadyReviewed) {
		t.Fatalf("expected ErrAlreadyReviewed on a second review, got %v", err)
	}
}

func TestCriticalSignalAutoEscalatesToACase(t *testing.T) {
	svc := newService(fakeModerator{admins: map[string]bool{"admin": true}}, true)
	ctx := context.Background()
	if _, err := svc.RecordSignal(ctx, trustsafety.SignalInput{ResourceType: "user", ResourceID: "u9", SignalType: "rapid_account_creation", Severity: trustsafety.SeverityLow}); err != nil {
		t.Fatalf("low signal: %v", err)
	}
	cases, err := svc.ListCases(ctx, "admin", "")
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	if len(cases) != 0 {
		t.Fatalf("expected a low-severity signal not to open a case, got %d cases", len(cases))
	}
	if _, err := svc.RecordSignal(ctx, trustsafety.SignalInput{ResourceType: "user", ResourceID: "u9", SignalType: "credential_stuffing", Severity: trustsafety.SeverityCritical}); err != nil {
		t.Fatalf("critical signal: %v", err)
	}
	cases, err = svc.ListCases(ctx, "admin", "")
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected a critical signal to auto-open exactly one case, got %d", len(cases))
	}
}

func TestIsRestrictedFalseForAnUnrestrictedUser(t *testing.T) {
	svc := newService(fakeModerator{}, true)
	restricted, reason, err := svc.IsRestricted(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("is restricted: %v", err)
	}
	if restricted || reason != "" {
		t.Fatalf("expected an unrestricted user, got restricted=%v reason=%q", restricted, reason)
	}
}
