// Package handlers has the built-in job types jobrunner.Runner ships
// with. Both are genuine, not simulated: Echo really logs and always
// succeeds (an honest smoke-test primitive, like notifications'
// LogSender is honest about not reaching a real device); Webhook really
// makes an HTTP call and really can fail (wrong URL, connection refused,
// timeout, non-2xx), which is what makes retry/backoff/dead-letter
// something this phase's live validation can prove against a genuine
// failure, not a fabricated one.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Echo logs the payload and always succeeds - useful for exercising the
// immediate/scheduled/delayed/recurring paths without needing any
// external dependency to cooperate.
func Echo(logger *slog.Logger) func(ctx context.Context, payload map[string]any) error {
	return func(ctx context.Context, payload map[string]any) error {
		logger.Info("jobrunner: echo", "payload", payload)
		return nil
	}
}

const webhookTimeout = 10 * time.Second

// Webhook POSTs payload["body"] (or the whole payload if "body" is
// absent) as JSON to payload["url"]. A missing/invalid URL, a connection
// failure, or a non-2xx response are all real, distinguishable failures -
// exactly the generic "call this endpoint, retry if it doesn't work"
// capability a background job queue exists for.
func Webhook(client *http.Client) func(ctx context.Context, payload map[string]any) error {
	if client == nil {
		client = &http.Client{Timeout: webhookTimeout}
	}
	return func(ctx context.Context, payload map[string]any) error {
		url, _ := payload["url"].(string)
		if url == "" {
			return fmt.Errorf("webhook job payload missing required \"url\" field")
		}
		body := payload["body"]
		if body == nil {
			body = payload
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal webhook body: %w", err)
		}

		reqCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(encoded))
		if err != nil {
			return fmt.Errorf("build webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("webhook request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook returned non-2xx status %d", resp.StatusCode)
		}
		return nil
	}
}
