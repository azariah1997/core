// Package memory is an in-memory devices.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/devices"
)

type Repository struct {
	mu   sync.Mutex
	byID map[string]devices.Device
	// key is userID + "|" + clientDeviceID
	byClientID map[string]string
}

func New() *Repository {
	return &Repository{byID: map[string]devices.Device{}, byClientID: map[string]string{}}
}

func key(userID, clientDeviceID string) string { return userID + "|" + clientDeviceID }

func (r *Repository) Register(ctx context.Context, userID string, in devices.RegisterInput) (devices.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	k := key(userID, in.ClientDeviceID)
	if id, ok := r.byClientID[k]; ok {
		d := r.byID[id]
		d.Platform, d.OSVersion, d.AppVersion = in.Platform, in.OSVersion, in.AppVersion
		d.Locale, d.Timezone = in.Locale, in.Timezone
		if in.PushToken != "" {
			d.HasPushToken = true
		}
		d.SessionStatus = devices.SessionActive
		d.LastActiveAt = now
		r.byID[id] = d
		return d, nil
	}

	d := devices.Device{
		ID: uuid.NewString(), UserID: userID, ClientDeviceID: in.ClientDeviceID,
		Platform: in.Platform, OSVersion: in.OSVersion, AppVersion: in.AppVersion,
		Locale: in.Locale, Timezone: in.Timezone, HasPushToken: in.PushToken != "",
		SessionStatus: devices.SessionActive, LastActiveAt: now, CreatedAt: now,
	}
	r.byID[d.ID] = d
	r.byClientID[k] = d.ID
	return d, nil
}

func (r *Repository) List(ctx context.Context, userID string) ([]devices.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []devices.Device
	for _, d := range r.byID {
		if d.UserID == userID && d.SessionStatus == devices.SessionActive {
			list = append(list, d)
		}
	}
	return list, nil
}

func (r *Repository) Revoke(ctx context.Context, userID, deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[deviceID]
	if !ok || d.UserID != userID || d.SessionStatus != devices.SessionActive {
		return devices.ErrNotFound
	}
	d.SessionStatus = devices.SessionRevoked
	r.byID[deviceID] = d
	return nil
}
