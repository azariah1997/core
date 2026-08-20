// Package notifications is the platform's single entry point for sending
// a notification to a user - "applications should call the Core
// Notification Service, not FCM/APNs directly" is the roadmap's own
// framing. Channel dispatch (push/email/SMS/in-app/realtime) is an
// internal implementation detail behind Service.Send; callers never talk
// to a provider directly.
package notifications

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Channel is a closed set - the roadmap names exactly these five.
type Channel string

const (
	ChannelPush     Channel = "push"
	ChannelEmail    Channel = "email"
	ChannelSMS      Channel = "sms"
	ChannelInApp    Channel = "in_app"
	ChannelRealtime Channel = "realtime"
)

func (c Channel) valid() bool {
	switch c {
	case ChannelPush, ChannelEmail, ChannelSMS, ChannelInApp, ChannelRealtime:
		return true
	default:
		return false
	}
}

// Interruptive channels are the ones quiet hours suppress - push and SMS
// physically interrupt the user (a buzz, a lock-screen banner). in_app
// and realtime are passive/pull: they're simply there when the user looks,
// so quiet hours don't apply to them.
func (c Channel) Interruptive() bool {
	return c == ChannelPush || c == ChannelSMS
}

type Status string

const (
	StatusPending   Status = "pending"
	StatusSent      Status = "sent"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
	StatusDeferred  Status = "deferred" // suppressed by quiet hours
	StatusSkipped   Status = "skipped"  // suppressed by user preference
)

// Notification is the logical, potentially multi-channel request a caller
// makes. It maps to the new notification_requests table - the
// pre-existing scaffold table named "notifications" turned out to already
// be shaped as a single-channel delivery attempt (see
// 0011_notifications.sql), so it became NotificationDelivery's table
// instead of this one's.
type Notification struct {
	ID       string
	AppID    string
	UserID   string
	Category string // product-defined, e.g. "message", "friend_request" - free-form
	Title    string
	Body     string
	Data     map[string]any
	Channels []Channel
	// SentByUserID is the caller who triggered this send - almost
	// always different from UserID now that Send allows any
	// authenticated caller to notify a different user (see Service.Send
	// and this module's README for why). Kept explicit rather than left
	// implicit, since "who actually did this" stops being derivable
	// from UserID alone once cross-user sends are allowed.
	SentByUserID string
	CreatedAt    time.Time
}

// NotificationTemplate lets a caller send by TemplateKey + variables
// instead of literal text every time. Rendered with Go's text/template,
// one Title/Body pair shared across every channel a send targets -
// per-channel template variants (e.g. a shorter SMS body) are a
// deliberate scope-down, deferred until a real need for them shows up.
type NotificationTemplate struct {
	ID            string
	AppID         string
	Key           string // product-defined, e.g. "message.new"
	TitleTemplate string
	BodyTemplate  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NotificationPreference is an opt-out model: the absence of a row for a
// (user, app, category, channel) means enabled. Only explicit overrides
// are stored.
type NotificationPreference struct {
	UserID   string
	AppID    string
	Category string
	Channel  Channel
	Enabled  bool
}

// QuietHours is per (user, app), not per category - it's a blanket "don't
// interrupt me" window. StartMinute/EndMinute are minutes since local
// midnight in Timezone; Start > End means the window wraps past midnight
// (e.g. 22:00-07:00).
type QuietHours struct {
	UserID      string
	AppID       string
	Timezone    string
	StartMinute int
	EndMinute   int
	Enabled     bool
}

// Active reports whether at is within the quiet hours window, expressed
// in Timezone's local wall-clock time.
func (q QuietHours) Active(at time.Time) bool {
	if !q.Enabled {
		return false
	}
	loc, err := time.LoadLocation(q.Timezone)
	if err != nil {
		loc = time.UTC
	}
	local := at.In(loc)
	minute := local.Hour()*60 + local.Minute()
	if q.StartMinute <= q.EndMinute {
		return minute >= q.StartMinute && minute < q.EndMinute
	}
	// Wraps past midnight.
	return minute >= q.StartMinute || minute < q.EndMinute
}

// NotificationDelivery is one channel's attempt/outcome for a
// Notification - maps to the pre-existing "notifications" table.
type NotificationDelivery struct {
	ID             string
	NotificationID string
	Channel        Channel
	Status         Status
	ProviderRef    string
	Error          string
	Attempts       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeliveredAt    *time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound         = errors.New("notification not found")
	ErrDeliveryNotFound = errors.New("delivery not found")
	ErrTemplateNotFound = errors.New("template not found")
	ErrTemplateKeyTaken = errors.New("a template with this key already exists for this app")
	ErrForbidden        = errors.New("not permitted to access this notification")
)

type SendInput struct {
	AppID        string
	UserID       string
	Category     string
	Channels     []Channel
	TemplateKey  string
	TemplateData map[string]any
	Title        string
	Body         string
	Data         map[string]any
}

func (in SendInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.UserID) == "":
		return &ValidationError{"userId is required"}
	case strings.TrimSpace(in.Category) == "":
		return &ValidationError{"category is required"}
	case len(in.Channels) == 0:
		return &ValidationError{"at least one channel is required"}
	}
	for _, ch := range in.Channels {
		if !ch.valid() {
			return &ValidationError{"channel must be one of push, email, sms, in_app, realtime"}
		}
	}
	if in.TemplateKey == "" && (strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Body) == "") {
		return &ValidationError{"either templateKey or both title and body are required"}
	}
	return nil
}

type CreateTemplateInput struct {
	AppID         string
	Key           string
	TitleTemplate string
	BodyTemplate  string
}

func (in CreateTemplateInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Key) == "":
		return &ValidationError{"key is required"}
	case strings.TrimSpace(in.TitleTemplate) == "":
		return &ValidationError{"titleTemplate is required"}
	case strings.TrimSpace(in.BodyTemplate) == "":
		return &ValidationError{"bodyTemplate is required"}
	}
	return nil
}

type UpdateTemplateInput struct {
	TitleTemplate *string
	BodyTemplate  *string
}

func (in UpdateTemplateInput) Validate() error {
	if in.TitleTemplate == nil && in.BodyTemplate == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	return nil
}

type SetPreferenceInput struct {
	Category string
	Channel  Channel
	Enabled  bool
}

func (in SetPreferenceInput) Validate() error {
	if strings.TrimSpace(in.Category) == "" {
		return &ValidationError{"category is required"}
	}
	if !in.Channel.valid() {
		return &ValidationError{"channel must be one of push, email, sms, in_app, realtime"}
	}
	return nil
}

type SetQuietHoursInput struct {
	Timezone    string
	StartMinute int
	EndMinute   int
	Enabled     bool
}

func (in SetQuietHoursInput) Validate() error {
	if strings.TrimSpace(in.Timezone) == "" {
		return &ValidationError{"timezone is required"}
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return &ValidationError{"timezone must be a valid IANA timezone name"}
	}
	if in.StartMinute < 0 || in.StartMinute >= 1440 || in.EndMinute < 0 || in.EndMinute >= 1440 {
		return &ValidationError{"startMinute and endMinute must be between 0 and 1439"}
	}
	return nil
}

type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []Notification
	NextCursor string
}

// Repository is the storage-agnostic boundary. Implementations own
// emitting notification.requested/sent/delivered/failed outbox events
// atomically with the writes that produce them, the same pattern as
// every other domain module's Repository.
type Repository interface {
	CreateNotification(ctx context.Context, sentByUserID string, in SendInput, title, body string) (Notification, error)
	GetNotification(ctx context.Context, id string) (Notification, error)
	ListNotificationsForUser(ctx context.Context, userID string, params ListParams) (ListResult, error)

	CreateDelivery(ctx context.Context, n Notification, channel Channel) (NotificationDelivery, error)
	UpdateDeliveryResult(ctx context.Context, deliveryID string, status Status, providerRef, errMsg string) (NotificationDelivery, error)
	GetDelivery(ctx context.Context, id string) (NotificationDelivery, error)
	ListDeliveries(ctx context.Context, notificationID string) ([]NotificationDelivery, error)

	GetTemplate(ctx context.Context, appID, key string) (NotificationTemplate, error)
	CreateTemplate(ctx context.Context, in CreateTemplateInput) (NotificationTemplate, error)
	UpdateTemplate(ctx context.Context, appID, key string, in UpdateTemplateInput) (NotificationTemplate, error)

	GetPreferences(ctx context.Context, userID, appID string) ([]NotificationPreference, error)
	SetPreference(ctx context.Context, userID, appID string, in SetPreferenceInput) (NotificationPreference, error)

	GetQuietHours(ctx context.Context, userID, appID string) (QuietHours, error)
	SetQuietHours(ctx context.Context, userID, appID string, in SetQuietHoursInput) (QuietHours, error)
}
