// Package coresdk is the Go client for Core Platform's HTTP API - the
// SDK "future products should primarily interact with Core through"
// (the roadmap's own words for this phase). It owns every cross-cutting
// concern a product would otherwise reimplement per-caller: auth and
// token refresh (auth.go), retries where safe (retry.go), correlation
// ID propagation, typed errors matching the platform's real envelope
// (errors.go), cursor pagination (pagination.go), and a realtime
// WebSocket client (realtime.go). Typed convenience methods for a
// representative slice of the real API live in operations.go; Do is
// the escape hatch for everything else - see operations.go's own
// header comment for which endpoints got a typed wrapper and why.
package coresdk

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const correlationIDHeader = "X-Correlation-ID"

// Client is the one entry point into core-api. Every method call
// attaches a real Bearer token (via TokenSource), a correlation ID, and
// applies the configured retry policy - a caller never touches
// net/http directly.
type Client struct {
	baseURL    string
	httpClient *http.Client
	tokens     TokenSource
	retry      retryPolicy
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client (e.g. to set a
// custom Timeout or Transport). Defaults to http.DefaultClient's
// settings via a fresh client with a 30s timeout.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithTokenSource sets how the client obtains a Bearer token. Required
// for every route except GET /platform and GET /livez /readyz /healthz.
func WithTokenSource(ts TokenSource) Option {
	return func(cl *Client) { cl.tokens = ts }
}

// WithRetries overrides the default retry policy (3 attempts, GET only,
// exponential backoff starting at 200ms). Pass maxAttempts=1 to disable
// retries entirely.
func WithRetries(maxAttempts int, baseDelay time.Duration) Option {
	return func(cl *Client) { cl.retry = retryPolicy{maxAttempts: maxAttempts, baseDelay: baseDelay} }
}

// NewClient builds a Client for the core-api instance at baseURL (e.g.
// "http://localhost:8080").
func NewClient(baseURL string, opts ...Option) *Client {
	cl := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retry:      defaultRetryPolicy(),
	}
	for _, opt := range opts {
		opt(cl)
	}
	return cl
}

// requestOptions are per-call overrides, currently just an explicit
// correlation ID - almost every call lets the client generate one.
type requestOptions struct {
	correlationID string
}

// RequestOption customizes a single Do call.
type RequestOption func(*requestOptions)

// WithCorrelationID propagates a caller-supplied correlation ID instead
// of letting the client generate one - useful when a request is part of
// a larger traced operation that already has an ID.
func WithCorrelationID(id string) RequestOption {
	return func(o *requestOptions) { o.correlationID = id }
}

// Do makes a single API call to path (e.g. "/v1/apps/"+id), JSON-encoding
// body (nil for none) and decoding a 2xx response into out (nil to
// discard the body). It is the one place every typed operation in
// operations.go ultimately calls, and the escape hatch for any route
// that doesn't have a typed wrapper yet.
func (c *Client) Do(ctx context.Context, method, path string, body, out any, opts ...RequestOption) error {
	var reqOpts requestOptions
	for _, opt := range opts {
		opt(&reqOpts)
	}
	correlationID := reqOpts.correlationID
	if correlationID == "" {
		correlationID = newCorrelationID()
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("coresdk: marshal request body: %w", err)
		}
	}

	attempts := c.retry.maxAttemptsFor(method)
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retry.delay(attempt)):
			}
		}

		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
		if err != nil {
			return fmt.Errorf("coresdk: build request: %w", err)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set(correlationIDHeader, correlationID)
		if c.tokens != nil {
			token, err := c.tokens.Token(ctx)
			if err != nil {
				return fmt.Errorf("coresdk: obtain token: %w", err)
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue // network error - eligible for retry per policy
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("coresdk: read response body: %w", err)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, out); err != nil {
					return fmt.Errorf("coresdk: decode response body: %w", err)
				}
			}
			return nil
		}

		apiErr := decodeAPIError(resp.StatusCode, respBody)
		if !c.retry.retryableStatus(resp.StatusCode) {
			return apiErr
		}
		lastErr = apiErr
	}
	return lastErr
}

func decodeAPIError(status int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: status}
	if err := json.Unmarshal(body, apiErr); err != nil || apiErr.Code == "" {
		apiErr.Code = CodeInternal
		apiErr.Message = strings.TrimSpace(string(body))
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(status)
		}
	}
	return apiErr
}

func newCorrelationID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
