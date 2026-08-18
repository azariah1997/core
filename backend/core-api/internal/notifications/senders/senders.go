// Package senders implements notifications.ChannelSender for each of the
// five channels. There is no real APNs/FCM/SES/SMS integration here - the
// roadmap's "Cloud Rule" requires local development to remain possible
// without paying for production infrastructure, and unlike Postgres/Redis/
// Keycloak/OpenFGA (all of which have real local-equivalent servers this
// repo already runs), push/email/SMS providers don't have an honest local
// stand-in. LogSender is that stand-in: it logs what would have been sent
// and reports success, structured so a real provider client can implement
// the same ChannelSender interface later without changing anything else.
// realtime and in-app are real, though - no external provider needed for
// either.
package senders

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
)

// LogSender is the local/dev stand-in for a real provider (APNs, FCM,
// SES, an SMS gateway). It always "succeeds" - there is no local queue or
// server that could genuinely fail to deliver something because nothing
// is actually being delivered anywhere. Kept honest for local dev, not a
// pretense that it reaches a real device/inbox/phone.
type LogSender struct {
	Channel notifications.Channel
	Logger  *slog.Logger
}

func (s LogSender) Send(ctx context.Context, n notifications.Notification) (string, error) {
	s.Logger.Info("notifications: local dev provider stand-in - not actually delivered",
		"channel", s.Channel, "userId", n.UserID, "category", n.Category, "title", n.Title)
	return "log:" + n.ID, nil
}

// DeviceLookup is the narrow surface PushSender needs from the devices
// domain - implemented directly by devices.Service in production. Kept
// this narrow (rather than depending on devices.Service's full interface)
// so notifications stays decoupled from devices beyond exactly this one
// question, the same consumer-defined-interface pattern as
// users.AccessChecker.
type DeviceLookup interface {
	HasPushToken(ctx context.Context, userID string) (bool, error)
}

// PushSender is a genuine, live-testable integration point even without a
// real APNs/FCM account: it checks whether the recipient has actually
// registered a device with a push token (Phase 5) before "sending".
// Someone with zero registered devices gets a real failure, not a
// simulated one - and registering a device then retrying the delivery
// (Service.RetryDelivery) genuinely succeeds afterward.
type PushSender struct {
	Devices DeviceLookup
	Logger  *slog.Logger
}

func (s PushSender) Send(ctx context.Context, n notifications.Notification) (string, error) {
	hasToken, err := s.Devices.HasPushToken(ctx, n.UserID)
	if err != nil {
		return "", err
	}
	if !hasToken {
		return "", errNoPushToken
	}
	s.Logger.Info("notifications: local dev provider stand-in - not actually delivered",
		"channel", "push", "userId", n.UserID, "category", n.Category, "title", n.Title)
	return "log:" + n.ID, nil
}

type noPushTokenError struct{}

func (noPushTokenError) Error() string { return "recipient has no device with a registered push token" }

var errNoPushToken = noPushTokenError{}

// Realtime is the narrow publish surface RealtimeSender needs - satisfied
// directly by platformkit/rtbus.Publisher, the same interface shape
// messaging.Service depends on (kept as a separate declaration rather
// than importing messaging's, per this repo's preference for each
// consumer defining its own narrow interface rather than sharing one
// across unrelated domain modules).
type Realtime interface {
	ToUser(ctx context.Context, userID string, payload json.RawMessage) error
}

// RealtimeSender pushes over the same Redis pub/sub bus messaging.Service
// uses (Phase 11) - a connected client receives it on its WebSocket with
// no action required, real and live-testable exactly like a chat message.
type RealtimeSender struct {
	Realtime Realtime
}

func (s RealtimeSender) Send(ctx context.Context, n notifications.Notification) (string, error) {
	payload, err := marshalRealtimeEvent(n)
	if err != nil {
		return "", err
	}
	if err := s.Realtime.ToUser(ctx, n.UserID, payload); err != nil {
		return "", err
	}
	return "realtime:" + n.ID, nil
}

func marshalRealtimeEvent(n notifications.Notification) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type": "notification", "id": n.ID, "category": n.Category,
		"title": n.Title, "body": n.Body, "data": n.Data,
	})
}

// InAppSender's "delivery" is trivial by design: the Notification row
// itself, already durably written before any sender runs, is the in-app
// inbox entry (see Service.ListMine / GET /v1/notifications). This Send
// call exists only so the in_app channel produces a NotificationDelivery
// row with the same shape as every other channel, not because there's a
// separate delivery step to perform.
type InAppSender struct{}

func (InAppSender) Send(ctx context.Context, n notifications.Notification) (string, error) {
	return "in_app:" + n.ID, nil
}
