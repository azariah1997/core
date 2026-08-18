package notifications

import (
	"bytes"
	"context"
	"log/slog"
	"text/template"
	"time"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// ChannelSender delivers one Notification over one channel. Registered
// per Channel in Service - callers of Service.Send never know or care
// which concrete provider (or local dev stand-in) handled delivery.
type ChannelSender interface {
	Send(ctx context.Context, n Notification) (providerRef string, err error)
}

// AdminChecker is satisfied directly by *authz.Service (identical method
// signature) - no adapter needed, unlike DeviceLookup below. Template
// management is gated on platform.admin: templates are cross-cutting app
// configuration, not a per-user action.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	repo    Repository
	senders map[Channel]ChannelSender
	admin   AdminChecker
	logger  *slog.Logger
}

func NewService(repo Repository, senders map[Channel]ChannelSender, admin AdminChecker, logger *slog.Logger) *Service {
	return &Service{repo: repo, senders: senders, admin: admin, logger: logger}
}

// Send is the platform's single notification entry point - "applications
// should call the Core Notification Service, not FCM/APNs directly."
// Access is self-or-platform.admin: a user can always notify themselves,
// and only a platform.admin can trigger a notification to someone else.
// A real "service account" concept (letting a product backend notify any
// user without being an admin) would be the natural next step here, but
// this platform has no caller identity distinct from "a user" yet, so
// self-or-admin is the safe default until that exists.
func (s *Service) Send(ctx context.Context, callerID string, in SendInput) (Notification, []NotificationDelivery, error) {
	if callerID != in.UserID {
		isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
		if err != nil {
			return Notification{}, nil, err
		}
		if !isAdmin {
			return Notification{}, nil, ErrForbidden
		}
	}
	if err := in.Validate(); err != nil {
		return Notification{}, nil, err
	}

	title, body := in.Title, in.Body
	if in.TemplateKey != "" {
		tmpl, err := s.repo.GetTemplate(ctx, in.AppID, in.TemplateKey)
		if err != nil {
			return Notification{}, nil, err
		}
		title, err = render(tmpl.TitleTemplate, in.TemplateData)
		if err != nil {
			return Notification{}, nil, &ValidationError{"failed to render title template: " + err.Error()}
		}
		body, err = render(tmpl.BodyTemplate, in.TemplateData)
		if err != nil {
			return Notification{}, nil, &ValidationError{"failed to render body template: " + err.Error()}
		}
	}

	n, err := s.repo.CreateNotification(ctx, in, title, body)
	if err != nil {
		return Notification{}, nil, err
	}

	prefs, err := s.repo.GetPreferences(ctx, in.UserID, in.AppID)
	if err != nil {
		return Notification{}, nil, err
	}
	disabled := map[Channel]bool{}
	for _, p := range prefs {
		if !p.Enabled && p.Category == in.Category {
			disabled[p.Channel] = true
		}
	}
	quiet, err := s.repo.GetQuietHours(ctx, in.UserID, in.AppID)
	if err != nil {
		return Notification{}, nil, err
	}
	inQuietHours := quiet.Active(time.Now())

	deliveries := make([]NotificationDelivery, 0, len(in.Channels))
	for _, ch := range in.Channels {
		d, err := s.repo.CreateDelivery(ctx, n, ch)
		if err != nil {
			return Notification{}, nil, err
		}
		switch {
		case disabled[ch]:
			d, err = s.repo.UpdateDeliveryResult(ctx, d.ID, StatusSkipped, "", "user has disabled this channel for this category")
		case ch.Interruptive() && inQuietHours:
			d, err = s.repo.UpdateDeliveryResult(ctx, d.ID, StatusDeferred, "", "suppressed by quiet hours")
		default:
			d = s.dispatch(ctx, n, d)
		}
		if err != nil {
			return Notification{}, nil, err
		}
		deliveries = append(deliveries, d)
	}
	return n, deliveries, nil
}

func (s *Service) dispatch(ctx context.Context, n Notification, d NotificationDelivery) NotificationDelivery {
	sender, ok := s.senders[d.Channel]
	if !ok {
		updated, err := s.repo.UpdateDeliveryResult(ctx, d.ID, StatusFailed, "", "no sender configured for this channel")
		if err != nil {
			s.logger.Error("notifications: failed to record missing-sender failure", "error", err, "deliveryId", d.ID)
			return d
		}
		return updated
	}
	ref, sendErr := sender.Send(ctx, n)
	status, errMsg := StatusSent, ""
	if sendErr != nil {
		status, errMsg = StatusFailed, sendErr.Error()
	}
	updated, err := s.repo.UpdateDeliveryResult(ctx, d.ID, status, ref, errMsg)
	if err != nil {
		s.logger.Error("notifications: failed to record delivery result", "error", err, "deliveryId", d.ID)
		return d
	}
	return updated
}

func render(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("notification").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Notification, error) {
	n, err := s.repo.GetNotification(ctx, id)
	if err != nil {
		return Notification{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, n.UserID); err != nil {
		return Notification{}, err
	}
	return n, nil
}

func (s *Service) ListMine(ctx context.Context, callerID string, params ListParams) (ListResult, error) {
	if params.Limit <= 0 || params.Limit > maxListLimit {
		params.Limit = defaultListLimit
	}
	return s.repo.ListNotificationsForUser(ctx, callerID, params)
}

func (s *Service) ListDeliveries(ctx context.Context, callerID, notificationID string) ([]NotificationDelivery, error) {
	n, err := s.repo.GetNotification(ctx, notificationID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, n.UserID); err != nil {
		return nil, err
	}
	return s.repo.ListDeliveries(ctx, notificationID)
}

// RetryDelivery re-invokes the channel sender for a delivery that
// previously failed or was deferred, incrementing its attempt count. A
// notification whose recipient later fixes the underlying problem (e.g.
// registers a device with a push token) can be retried without the
// platform needing a background scheduler in this phase - see the
// package README for why that's a deliberate, documented scope-down.
func (s *Service) RetryDelivery(ctx context.Context, callerID, notificationID, deliveryID string) (NotificationDelivery, error) {
	n, err := s.repo.GetNotification(ctx, notificationID)
	if err != nil {
		return NotificationDelivery{}, err
	}
	if err := s.requireOwnerOrAdmin(ctx, callerID, n.UserID); err != nil {
		return NotificationDelivery{}, err
	}
	d, err := s.repo.GetDelivery(ctx, deliveryID)
	if err != nil {
		return NotificationDelivery{}, err
	}
	if d.NotificationID != notificationID {
		return NotificationDelivery{}, ErrDeliveryNotFound
	}
	return s.dispatch(ctx, n, d), nil
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}

func (s *Service) CreateTemplate(ctx context.Context, callerID string, in CreateTemplateInput) (NotificationTemplate, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return NotificationTemplate{}, err
	}
	if err := in.Validate(); err != nil {
		return NotificationTemplate{}, err
	}
	return s.repo.CreateTemplate(ctx, in)
}

func (s *Service) UpdateTemplate(ctx context.Context, callerID, appID, key string, in UpdateTemplateInput) (NotificationTemplate, error) {
	if err := s.requireAdmin(ctx, callerID); err != nil {
		return NotificationTemplate{}, err
	}
	if err := in.Validate(); err != nil {
		return NotificationTemplate{}, err
	}
	return s.repo.UpdateTemplate(ctx, appID, key, in)
}

func (s *Service) GetTemplate(ctx context.Context, appID, key string) (NotificationTemplate, error) {
	return s.repo.GetTemplate(ctx, appID, key)
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

func (s *Service) GetPreferences(ctx context.Context, callerID, appID string) ([]NotificationPreference, error) {
	return s.repo.GetPreferences(ctx, callerID, appID)
}

func (s *Service) SetPreference(ctx context.Context, callerID, appID string, in SetPreferenceInput) (NotificationPreference, error) {
	if err := in.Validate(); err != nil {
		return NotificationPreference{}, err
	}
	return s.repo.SetPreference(ctx, callerID, appID, in)
}

func (s *Service) GetQuietHours(ctx context.Context, callerID, appID string) (QuietHours, error) {
	return s.repo.GetQuietHours(ctx, callerID, appID)
}

func (s *Service) SetQuietHours(ctx context.Context, callerID, appID string, in SetQuietHoursInput) (QuietHours, error) {
	if err := in.Validate(); err != nil {
		return QuietHours{}, err
	}
	return s.repo.SetQuietHours(ctx, callerID, appID, in)
}
