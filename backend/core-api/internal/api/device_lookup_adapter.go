package api

import (
	"context"

	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/notifications/senders"
)

// devicePushTokenLookup implements senders.DeviceLookup using
// devices.Service - notifications/senders depends only on this one
// narrow question, not devices.Service's full interface.
type devicePushTokenLookup struct {
	devicesSvc *devices.Service
}

// NewDevicePushTokenLookup is exported for cmd/server/main.go to wire
// into notifications' PushSender.
func NewDevicePushTokenLookup(devicesSvc *devices.Service) senders.DeviceLookup {
	return &devicePushTokenLookup{devicesSvc: devicesSvc}
}

func (l *devicePushTokenLookup) HasPushToken(ctx context.Context, userID string) (bool, error) {
	list, err := l.devicesSvc.List(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, d := range list {
		if d.HasPushToken {
			return true, nil
		}
	}
	return false, nil
}
