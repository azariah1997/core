// Package trustsafety implements Phase 21: Block, Mute, Report,
// ModerationCase, Suspension, Ban, Appeal, plus rate limiting, spam
// protection, and abuse signals.
//
// Block already exists - Phase 8's relationships module (a
// relationship's Status can be "blocked") already is the platform's
// reusable block primitive, and isn't duplicated here. Everything else
// in the roadmap's list is genuinely new and lives in this package.
package trustsafety

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Mute is one-directional and silent - the muted user is never
// notified and isn't prevented from interacting with the muter; only
// the muter stops seeing them. Unlike Block (a full relationship state
// both sides can observe), this needs no product-facing "why" beyond
// "I don't want to see this person," so there's no Reason field.
type Mute struct {
	ID          string
	MuterUserID string
	MutedUserID string
	CreatedAt   time.Time
}

type ReportStatus string

const (
	ReportStatusOpen      ReportStatus = "open"
	ReportStatusReviewing ReportStatus = "reviewing"
	ReportStatusResolved  ReportStatus = "resolved"
	ReportStatusDismissed ReportStatus = "dismissed"
)

// Report's Reason is free-form and product-defined - "design so
// products can supply product-specific report reasons" is the
// roadmap's own instruction, the same convention as RelationshipType
// (Phase 8) and Message.Type (Phase 11): never a fixed platform enum.
// ResourceType/ResourceID generalize to reporting anything (a user, a
// message, a file...) - the same generic resource-addressing pattern
// audit.Record (Phase 19) already uses.
type Report struct {
	ID             string
	ReporterUserID string
	ResourceType   string
	ResourceID     string
	Reason         string
	Details        string
	CaseID         string
	Status         ReportStatus
	CreatedAt      time.Time
}

type ReportInput struct {
	ResourceType string
	ResourceID   string
	Reason       string
	Details      string
}

func (in ReportInput) Validate() error {
	if strings.TrimSpace(in.ResourceType) == "" {
		return &ValidationError{"resourceType is required"}
	}
	if strings.TrimSpace(in.ResourceID) == "" {
		return &ValidationError{"resourceId is required"}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return &ValidationError{"reason is required"}
	}
	return nil
}

type CaseStatus string

const (
	CaseStatusOpen      CaseStatus = "open"
	CaseStatusInReview  CaseStatus = "in_review"
	CaseStatusResolved  CaseStatus = "resolved"
	CaseStatusDismissed CaseStatus = "dismissed"
)

// ModerationCase is the central review record every Report and
// escalated AbuseSignal against the same resource funnels into - one
// case stays open per (ResourceType, ResourceID) at a time (enforced by
// a partial unique index in the migration), so ten reports about the
// same message become one case with ReportCount 10, not ten separate
// cases for a moderator to deduplicate by hand.
type ModerationCase struct {
	ID           string
	ResourceType string
	ResourceID   string
	Status       CaseStatus
	AssignedTo   string
	Resolution   string
	ReportCount  int
	CreatedAt    time.Time
	ResolvedAt   *time.Time
}

type ResolveCaseInput struct {
	Status     CaseStatus
	Resolution string
}

func (in ResolveCaseInput) Validate() error {
	switch in.Status {
	case CaseStatusResolved, CaseStatusDismissed:
		return nil
	default:
		return &ValidationError{"status must be resolved or dismissed"}
	}
}

// Suspension is temporary - EndsAt is required. IsActive is computed at
// query time (now between StartsAt and EndsAt, and never lifted early)
// rather than stored as a mutable flag a scheduler would need to flip
// when time passes - the same "don't invent scheduler infrastructure
// this phase doesn't need" principle as Phase 13's PurgeExpired and
// Phase 20's retention policies.
type Suspension struct {
	ID        string
	UserID    string
	Reason    string
	CaseID    string
	IssuedBy  string
	StartsAt  time.Time
	EndsAt    time.Time
	LiftedAt  *time.Time
	LiftedBy  string
	CreatedAt time.Time
}

func (s Suspension) IsActive(now time.Time) bool {
	return s.LiftedAt == nil && now.Before(s.EndsAt)
}

type SuspendInput struct {
	UserID   string
	Reason   string
	CaseID   string
	Duration time.Duration
}

func (in SuspendInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return &ValidationError{"userId is required"}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return &ValidationError{"reason is required"}
	}
	if in.Duration <= 0 {
		return &ValidationError{"duration must be positive"}
	}
	return nil
}

// Ban has no EndsAt at all - permanent until explicitly lifted via
// LiftedAt, the same lift mechanism Suspension uses.
type Ban struct {
	ID        string
	UserID    string
	Reason    string
	CaseID    string
	IssuedBy  string
	LiftedAt  *time.Time
	LiftedBy  string
	CreatedAt time.Time
}

func (b Ban) IsActive() bool { return b.LiftedAt == nil }

type BanInput struct {
	UserID string
	Reason string
	CaseID string
}

func (in BanInput) Validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return &ValidationError{"userId is required"}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return &ValidationError{"reason is required"}
	}
	return nil
}

type AppealTargetType string

const (
	AppealTargetSuspension AppealTargetType = "suspension"
	AppealTargetBan        AppealTargetType = "ban"
)

type AppealStatus string

const (
	AppealStatusPending  AppealStatus = "pending"
	AppealStatusApproved AppealStatus = "approved"
	AppealStatusDenied   AppealStatus = "denied"
)

// Appeal lets a suspended or banned user contest the decision -
// approving one lifts the target (Suspension or Ban) as part of the
// same review action, see Service.ReviewAppeal.
type Appeal struct {
	ID         string
	UserID     string
	TargetType AppealTargetType
	TargetID   string
	Reason     string
	Status     AppealStatus
	ReviewedBy string
	CreatedAt  time.Time
	ReviewedAt *time.Time
}

type AppealInput struct {
	TargetType AppealTargetType
	TargetID   string
	Reason     string
}

func (in AppealInput) Validate() error {
	if in.TargetType != AppealTargetSuspension && in.TargetType != AppealTargetBan {
		return &ValidationError{"targetType must be suspension or ban"}
	}
	if strings.TrimSpace(in.TargetID) == "" {
		return &ValidationError{"targetId is required"}
	}
	if strings.TrimSpace(in.Reason) == "" {
		return &ValidationError{"reason is required"}
	}
	return nil
}

// AbuseSignal is a lightweight, open-to-record observation - distinct
// from Report (a person reporting something) and from Phase 19's audit
// (a record of what happened): a signal is "something looks off about
// X," which may or may not ever need a human to look at it. SignalType
// is free-form (e.g. "rate_limit_exceeded", "rapid_account_creation"),
// matching every other Type-shaped field in this repo.
type AbuseSignal struct {
	ID           string
	ResourceType string
	ResourceID   string
	SignalType   string
	Severity     string
	Metadata     map[string]any
	RecordedAt   time.Time
}

type SignalSeverity string

const (
	SeverityLow      SignalSeverity = "low"
	SeverityMedium   SignalSeverity = "medium"
	SeverityHigh     SignalSeverity = "high"
	SeverityCritical SignalSeverity = "critical"
)

type SignalInput struct {
	ResourceType string
	ResourceID   string
	SignalType   string
	Severity     SignalSeverity
	Metadata     map[string]any
}

func (in SignalInput) Validate() error {
	if strings.TrimSpace(in.ResourceType) == "" {
		return &ValidationError{"resourceType is required"}
	}
	if strings.TrimSpace(in.ResourceID) == "" {
		return &ValidationError{"resourceId is required"}
	}
	if strings.TrimSpace(in.SignalType) == "" {
		return &ValidationError{"signalType is required"}
	}
	switch in.Severity {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
	default:
		return &ValidationError{"severity must be low, medium, high, or critical"}
	}
	return nil
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound        = errors.New("resource not found")
	ErrForbidden       = errors.New("not permitted to perform this trust & safety action")
	ErrAlreadyReviewed = errors.New("appeal has already been reviewed")
)

// Repository is the storage-agnostic boundary for everything in this
// package.
type Repository interface {
	Mute(ctx context.Context, m Mute) error
	Unmute(ctx context.Context, muterUserID, mutedUserID string) error
	ListMutes(ctx context.Context, muterUserID string) ([]Mute, error)

	// OpenOrAttachCase returns the currently open/in_review case for
	// (resourceType, resourceID), creating one if none exists, and
	// increments its ReportCount - the dedup logic the migration's
	// partial unique index makes safe to implement as "try insert,
	// fall back to select" without a race.
	OpenOrAttachCase(ctx context.Context, resourceType, resourceID string) (ModerationCase, error)
	GetCase(ctx context.Context, id string) (ModerationCase, error)
	ListCases(ctx context.Context, status CaseStatus) ([]ModerationCase, error)
	ResolveCase(ctx context.Context, id string, in ResolveCaseInput) (ModerationCase, error)
	AssignCase(ctx context.Context, id, assigneeUserID string) (ModerationCase, error)

	CreateReport(ctx context.Context, r Report) error
	GetReport(ctx context.Context, id string) (Report, error)
	ListReports(ctx context.Context, caseID string) ([]Report, error)

	CreateSuspension(ctx context.Context, s Suspension) error
	GetSuspension(ctx context.Context, id string) (Suspension, error)
	ListSuspensions(ctx context.Context, userID string) ([]Suspension, error)
	LiftSuspension(ctx context.Context, id, liftedBy string) (Suspension, error)

	CreateBan(ctx context.Context, b Ban) error
	GetBan(ctx context.Context, id string) (Ban, error)
	ListBans(ctx context.Context, userID string) ([]Ban, error)
	LiftBan(ctx context.Context, id, liftedBy string) (Ban, error)

	CreateAppeal(ctx context.Context, a Appeal) error
	GetAppeal(ctx context.Context, id string) (Appeal, error)
	ListAppeals(ctx context.Context, userID string) ([]Appeal, error)
	ReviewAppeal(ctx context.Context, id string, status AppealStatus, reviewedBy string) (Appeal, error)

	RecordSignal(ctx context.Context, sig AbuseSignal) error
	ListSignals(ctx context.Context, resourceType, resourceID string) ([]AbuseSignal, error)
}
