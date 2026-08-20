// Package core holds mood's adapter onto a real Core Platform
// capability - analytics ingest. Duplicated from pulse-interactions'
// own AnalyticsAdapter of the same shape (this codebase's consumer-
// defined-interface convention: every module owns its own adapter,
// even an identical one, rather than sharing) rather than importing it
// - see internal/mood/pulsemodules for mood's *other* adapters, onto
// sibling Pulse modules, which is why those live in a separately named
// package instead of here.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

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
