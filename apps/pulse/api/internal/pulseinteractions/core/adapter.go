// Package core holds pulseinteractions' real, production adapters onto
// Core Platform capabilities - relationships (per-caller), realtime
// delivery (fixed, via the shared Redis rtbus contract), and analytics
// (fixed, via Core's real unauthenticated Track endpoint).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/rtbus"
)

// RelationshipsAdapter adapts a per-caller *coresdk.Client onto
// pulseinteractions.CoreRelationships - only ListMine, unlike bond/
// pulse-connections' fuller adapters, since this module never mutates
// a relationship.
type RelationshipsAdapter struct {
	client *coresdk.Client
	appID  string
}

func NewRelationshipsAdapter(client *coresdk.Client, appID string) RelationshipsAdapter {
	return RelationshipsAdapter{client: client, appID: appID}
}

func (a RelationshipsAdapter) ListMine(ctx context.Context, relType string) ([]pulseinteractions.RelationshipRef, error) {
	var out struct {
		Items []struct {
			RequesterID string `json:"requesterUserId"`
			TargetID    string `json:"targetUserId"`
			Status      string `json:"status"`
		} `json:"items"`
	}
	path := "/v1/relationships?appId=" + url.QueryEscape(a.appID) + "&type=" + url.QueryEscape(relType)
	if err := a.client.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	refs := make([]pulseinteractions.RelationshipRef, 0, len(out.Items))
	for _, item := range out.Items {
		refs = append(refs, pulseinteractions.RelationshipRef{RequesterID: item.RequesterID, TargetID: item.TargetID, Status: item.Status})
	}
	return refs, nil
}

// RealtimeAdapter adapts platformkit/rtbus.Publisher onto
// pulseinteractions.Realtime - a fixed, service-level dependency
// constructed once at startup with the shared Redis client, publishing
// to the exact same channel realtime-gateway's hub subscribes to.
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
// POST /v1/analytics/events (the platform's one open write endpoint -
// see backend/core-api/internal/analytics/README.md) directly, never
// through a per-caller client, since Track needs no caller token.
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
// (Phase 5) using the caller's own bearer token - a second *coresdk.Client
// pointed at a different base URL than the one CoreFactory builds,
// since presence lives on realtime-gateway, not core-api.
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
// caller's own authenticated client - now that Send allows any
// authenticated caller to notify any user (see notifications/README.md),
// this is a real, attributed, cross-user push request, not a workaround.
type NotifierAdapter struct {
	client *coresdk.Client
	appID  string
}

func NewNotifierAdapter(client *coresdk.Client, appID string) NotifierAdapter {
	return NotifierAdapter{client: client, appID: appID}
}

func (a NotifierAdapter) NotifyPulseReceived(ctx context.Context, receiverUserID string, durationMs int) error {
	body := map[string]any{
		"appId": a.appID, "userId": receiverUserID, "category": "pulse_received",
		"channels": []string{"push"}, "title": "Pulse", "body": "You received a Pulse",
		"data": map[string]any{"durationMs": durationMs},
	}
	return a.client.Do(ctx, "POST", "/v1/notifications", body, nil)
}
