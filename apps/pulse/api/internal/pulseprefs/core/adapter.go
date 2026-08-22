// Package core holds pulseprefs' real, production adapters onto Core
// Platform capabilities - notifications.QuietHours and
// trustsafety.Mute, both genuinely shared Core capabilities this module
// never duplicates.
package core

import (
	"context"
	"net/url"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
	"github.com/example/core-platform/packages/go/coresdk"
)

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05.000Z07:00", s)
}

// QuietHoursAdapter calls Core's real GET/PUT
// /v1/notification-preferences/quiet-hours using the caller's own
// authenticated client - always Pulse's own AppID, never a
// client-supplied value, the same "never trust the client for appId"
// rule every other Core-scoped adapter in this codebase follows.
type QuietHoursAdapter struct {
	client *coresdk.Client
	appID  string
}

func NewQuietHoursAdapter(client *coresdk.Client, appID string) QuietHoursAdapter {
	return QuietHoursAdapter{client: client, appID: appID}
}

func (a QuietHoursAdapter) Get(ctx context.Context) (pulseprefs.QuietHours, error) {
	var out struct {
		Timezone    string `json:"timezone"`
		StartMinute int    `json:"startMinute"`
		EndMinute   int    `json:"endMinute"`
		Enabled     bool   `json:"enabled"`
	}
	path := "/v1/notification-preferences/quiet-hours?appId=" + url.QueryEscape(a.appID)
	if err := a.client.Do(ctx, "GET", path, nil, &out); err != nil {
		return pulseprefs.QuietHours{}, err
	}
	return pulseprefs.QuietHours{Timezone: out.Timezone, StartMinute: out.StartMinute, EndMinute: out.EndMinute, Enabled: out.Enabled}, nil
}

func (a QuietHoursAdapter) Set(ctx context.Context, in pulseprefs.SetQuietHoursInput) (pulseprefs.QuietHours, error) {
	body := map[string]any{
		"appId": a.appID, "timezone": in.Timezone, "startMinute": in.StartMinute, "endMinute": in.EndMinute, "enabled": in.Enabled,
	}
	var out struct {
		Timezone    string `json:"timezone"`
		StartMinute int    `json:"startMinute"`
		EndMinute   int    `json:"endMinute"`
		Enabled     bool   `json:"enabled"`
	}
	if err := a.client.Do(ctx, "PUT", "/v1/notification-preferences/quiet-hours", body, &out); err != nil {
		return pulseprefs.QuietHours{}, err
	}
	return pulseprefs.QuietHours{Timezone: out.Timezone, StartMinute: out.StartMinute, EndMinute: out.EndMinute, Enabled: out.Enabled}, nil
}

// MutesAdapter calls Core's real, platform-wide (not app-scoped)
// trustsafety mute endpoints using the caller's own authenticated
// client.
type MutesAdapter struct {
	client *coresdk.Client
}

func NewMutesAdapter(client *coresdk.Client) MutesAdapter {
	return MutesAdapter{client: client}
}

func (a MutesAdapter) Mute(ctx context.Context, mutedUserID string) (pulseprefs.Mute, error) {
	body := map[string]any{"mutedUserId": mutedUserID}
	var out struct {
		ID          string `json:"id"`
		MutedUserID string `json:"mutedUserId"`
		CreatedAt   string `json:"createdAt"`
	}
	if err := a.client.Do(ctx, "POST", "/v1/trustsafety/mutes", body, &out); err != nil {
		return pulseprefs.Mute{}, err
	}
	createdAt, _ := parseTime(out.CreatedAt)
	return pulseprefs.Mute{ID: out.ID, MutedUserID: out.MutedUserID, CreatedAt: createdAt}, nil
}

func (a MutesAdapter) List(ctx context.Context) ([]pulseprefs.Mute, error) {
	var out struct {
		Items []struct {
			ID          string `json:"id"`
			MutedUserID string `json:"mutedUserId"`
			CreatedAt   string `json:"createdAt"`
		} `json:"items"`
	}
	if err := a.client.Do(ctx, "GET", "/v1/trustsafety/mutes", nil, &out); err != nil {
		return nil, err
	}
	mutes := make([]pulseprefs.Mute, 0, len(out.Items))
	for _, item := range out.Items {
		createdAt, _ := parseTime(item.CreatedAt)
		mutes = append(mutes, pulseprefs.Mute{ID: item.ID, MutedUserID: item.MutedUserID, CreatedAt: createdAt})
	}
	return mutes, nil
}

func (a MutesAdapter) Unmute(ctx context.Context, mutedUserID string) error {
	return a.client.Do(ctx, "DELETE", "/v1/trustsafety/mutes/"+url.PathEscape(mutedUserID), nil, nil)
}
