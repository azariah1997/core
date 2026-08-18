// Package memory is an in-memory notifications.Repository for tests.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
)

type Repository struct {
	mu          sync.Mutex
	requests    map[string]notifications.Notification
	deliveries  map[string]notifications.NotificationDelivery
	templates   map[string]notifications.NotificationTemplate // appID|key
	preferences map[string]notifications.NotificationPreference
	quietHours  map[string]notifications.QuietHours // userID|appID
}

func New() *Repository {
	return &Repository{
		requests:    map[string]notifications.Notification{},
		deliveries:  map[string]notifications.NotificationDelivery{},
		templates:   map[string]notifications.NotificationTemplate{},
		preferences: map[string]notifications.NotificationPreference{},
		quietHours:  map[string]notifications.QuietHours{},
	}
}

func templateKey(appID, key string) string      { return appID + "|" + key }
func quietHoursKey(userID, appID string) string { return userID + "|" + appID }
func preferenceKey(userID, appID, category string, channel notifications.Channel) string {
	return userID + "|" + appID + "|" + category + "|" + string(channel)
}

func (r *Repository) CreateNotification(ctx context.Context, in notifications.SendInput, title, body string) (notifications.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := notifications.Notification{
		ID: uuid.NewString(), AppID: in.AppID, UserID: in.UserID, Category: in.Category,
		Title: title, Body: body, Data: in.Data, Channels: in.Channels, CreatedAt: time.Now().UTC(),
	}
	r.requests[n.ID] = n
	return n, nil
}

func (r *Repository) GetNotification(ctx context.Context, id string) (notifications.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.requests[id]
	if !ok {
		return notifications.Notification{}, notifications.ErrNotFound
	}
	return n, nil
}

func (r *Repository) ListNotificationsForUser(ctx context.Context, userID string, params notifications.ListParams) (notifications.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []notifications.Notification
	for _, n := range r.requests {
		if n.UserID == userID {
			all = append(all, n)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	start := 0
	if params.Cursor != "" {
		afterID, err := decodeCursor(params.Cursor)
		if err != nil {
			return notifications.ListResult{}, &notifications.ValidationError{Message: "invalid cursor"}
		}
		for i, n := range all {
			if n.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + params.Limit
	if end > len(all) {
		end = len(all)
	}
	if start > len(all) {
		start = len(all)
	}
	page := append([]notifications.Notification{}, all[start:end]...)

	result := notifications.ListResult{Items: page}
	if end < len(all) {
		result.NextCursor = encodeCursor(page[len(page)-1].ID)
	}
	return result, nil
}

func encodeCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func decodeCursor(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	return string(raw), err
}

func (r *Repository) CreateDelivery(ctx context.Context, n notifications.Notification, channel notifications.Channel) (notifications.NotificationDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	d := notifications.NotificationDelivery{
		ID: uuid.NewString(), NotificationID: n.ID, Channel: channel, Status: notifications.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	}
	r.deliveries[d.ID] = d
	return d, nil
}

func (r *Repository) UpdateDeliveryResult(ctx context.Context, deliveryID string, status notifications.Status, providerRef, errMsg string) (notifications.NotificationDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.deliveries[deliveryID]
	if !ok {
		return notifications.NotificationDelivery{}, notifications.ErrDeliveryNotFound
	}
	d.Status = status
	d.ProviderRef = providerRef
	d.Error = errMsg
	d.Attempts++
	d.UpdatedAt = time.Now().UTC()
	if status == notifications.StatusDelivered {
		now := time.Now().UTC()
		d.DeliveredAt = &now
	}
	r.deliveries[deliveryID] = d
	return d, nil
}

func (r *Repository) GetDelivery(ctx context.Context, id string) (notifications.NotificationDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.deliveries[id]
	if !ok {
		return notifications.NotificationDelivery{}, notifications.ErrDeliveryNotFound
	}
	return d, nil
}

func (r *Repository) ListDeliveries(ctx context.Context, notificationID string) ([]notifications.NotificationDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []notifications.NotificationDelivery
	for _, d := range r.deliveries {
		if d.NotificationID == notificationID {
			list = append(list, d)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) GetTemplate(ctx context.Context, appID, key string) (notifications.NotificationTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.templates[templateKey(appID, key)]
	if !ok {
		return notifications.NotificationTemplate{}, notifications.ErrTemplateNotFound
	}
	return t, nil
}

func (r *Repository) CreateTemplate(ctx context.Context, in notifications.CreateTemplateInput) (notifications.NotificationTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := templateKey(in.AppID, in.Key)
	if _, exists := r.templates[k]; exists {
		return notifications.NotificationTemplate{}, notifications.ErrTemplateKeyTaken
	}
	now := time.Now().UTC()
	t := notifications.NotificationTemplate{
		ID: uuid.NewString(), AppID: in.AppID, Key: in.Key,
		TitleTemplate: in.TitleTemplate, BodyTemplate: in.BodyTemplate,
		CreatedAt: now, UpdatedAt: now,
	}
	r.templates[k] = t
	return t, nil
}

func (r *Repository) UpdateTemplate(ctx context.Context, appID, key string, in notifications.UpdateTemplateInput) (notifications.NotificationTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := templateKey(appID, key)
	t, ok := r.templates[k]
	if !ok {
		return notifications.NotificationTemplate{}, notifications.ErrTemplateNotFound
	}
	if in.TitleTemplate != nil {
		t.TitleTemplate = *in.TitleTemplate
	}
	if in.BodyTemplate != nil {
		t.BodyTemplate = *in.BodyTemplate
	}
	t.UpdatedAt = time.Now().UTC()
	r.templates[k] = t
	return t, nil
}

func (r *Repository) GetPreferences(ctx context.Context, userID, appID string) ([]notifications.NotificationPreference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []notifications.NotificationPreference
	for _, p := range r.preferences {
		if p.UserID == userID && p.AppID == appID {
			list = append(list, p)
		}
	}
	return list, nil
}

func (r *Repository) SetPreference(ctx context.Context, userID, appID string, in notifications.SetPreferenceInput) (notifications.NotificationPreference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := notifications.NotificationPreference{UserID: userID, AppID: appID, Category: in.Category, Channel: in.Channel, Enabled: in.Enabled}
	r.preferences[preferenceKey(userID, appID, in.Category, in.Channel)] = p
	return p, nil
}

func (r *Repository) GetQuietHours(ctx context.Context, userID, appID string) (notifications.QuietHours, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.quietHours[quietHoursKey(userID, appID)]
	if !ok {
		return notifications.QuietHours{UserID: userID, AppID: appID, Timezone: "UTC"}, nil
	}
	return q, nil
}

func (r *Repository) SetQuietHours(ctx context.Context, userID, appID string, in notifications.SetQuietHoursInput) (notifications.QuietHours, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := notifications.QuietHours{
		UserID: userID, AppID: appID, Timezone: in.Timezone,
		StartMinute: in.StartMinute, EndMinute: in.EndMinute, Enabled: in.Enabled,
	}
	r.quietHours[quietHoursKey(userID, appID)] = q
	return q, nil
}
