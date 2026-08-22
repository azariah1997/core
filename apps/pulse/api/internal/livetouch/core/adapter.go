// Package core holds live-touch's real, production adapters onto Core
// Platform capabilities - presence (per-caller), notifications
// (per-caller), realtime delivery (fixed, via the shared Redis rtbus
// contract), and analytics (fixed, via Core's real unauthenticated
// Track endpoint). Duplicated from pulse-interactions' own adapters of
// the same shape rather than imported - this codebase's
// consumer-defined-interface convention.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/rtbus"
)

// RealtimeAdapter adapts platformkit/rtbus.Publisher onto
// livetouch.Realtime - a fixed, service-level dependency constructed
// once at startup with the shared Redis client.
type RealtimeAdapter struct {
	publisher *rtbus.Publisher
}

func NewRealtimeAdapter(publisher *rtbus.Publisher) RealtimeAdapter {
	return RealtimeAdapter{publisher: publisher}
}

func (a RealtimeAdapter) ToUser(ctx context.Context, userID string, payload []byte) error {
	return a.publisher.ToUser(ctx, userID, payload)
}

// AnalyticsAdapter calls Core's real, deliberately unauthenticated
// POST /v1/analytics/events directly, never through a per-caller
// client, since Track needs no caller token.
type AnalyticsAdapter struct {
	coreAPIURL string
	appID      string
	httpClient *http.Client
}

func NewAnalyticsAdapter(coreAPIURL, appID string) AnalyticsAdapter {
	return AnalyticsAdapter{coreAPIURL: strings.TrimRight(coreAPIURL, "/"), appID: appID, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (a AnalyticsAdapter) Track(ctx context.Context, eventName, userID string, properties map[string]any) error {
	body, err := json.Marshal(map[string]any{
		"events": []map[string]any{{
			"eventName": eventName, "userId": userID, "appId": a.appID, "properties": properties,
		}},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.coreAPIURL+"/v1/analytics/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// PresenceAdapter calls realtime-gateway's real GET /v1/presence/{userId}
// using the caller's own bearer token - a second *coresdk.Client pointed
// at a different base URL than the one core-api's factory builds.
type PresenceAdapter struct {
	client *coresdk.Client
}

func NewPresenceAdapter(realtimeAPIURL, token string) PresenceAdapter {
	client := coresdk.NewClient(realtimeAPIURL, coresdk.WithTokenSource(coresdk.StaticTokenSource(token)), coresdk.WithRetries(1, 0))
	return PresenceAdapter{client: client}
}

func (a PresenceAdapter) IsOnline(ctx context.Context, userID string) (bool, error) {
	var out struct {
		Online bool `json:"online"`
	}
	if err := a.client.Do(ctx, "GET", "/v1/presence/"+userID, nil, &out); err != nil {
		return false, err
	}
	return out.Online, nil
}

// NotifierAdapter calls Core's real POST /v1/notifications using the
// caller's own authenticated client.
type NotifierAdapter struct {
	client *coresdk.Client
	appID  string
}

func NewNotifierAdapter(client *coresdk.Client, appID string) NotifierAdapter {
	return NotifierAdapter{client: client, appID: appID}
}

func (a NotifierAdapter) Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error {
	reqBody := map[string]any{
		"appId": a.appID, "userId": receiverUserID, "category": category,
		"channels": []string{"push"}, "title": title, "body": body,
		"data": data,
	}
	return a.client.Do(ctx, "POST", "/v1/notifications", reqBody, nil)
}
