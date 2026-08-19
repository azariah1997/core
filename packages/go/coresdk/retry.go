package coresdk

import (
	"net/http"
	"time"
)

// retryPolicy implements "retries where safe" - literally: only GET is
// retried by default. Retrying a POST/PATCH/DELETE blindly risks a
// duplicate side effect (a second entitlement grant, a second message
// send) the server has no idempotency key to deduplicate against in
// most of this platform's routes, so the safe default is "don't."
// WithRetries lets a caller opt into a different policy if they know
// better for their own use (e.g. a route they know is idempotent).
type retryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{maxAttempts: 3, baseDelay: 200 * time.Millisecond}
}

func (p retryPolicy) maxAttemptsFor(method string) int {
	if method != http.MethodGet {
		return 1
	}
	if p.maxAttempts < 1 {
		return 1
	}
	return p.maxAttempts
}

func (p retryPolicy) delay(attempt int) time.Duration {
	d := p.baseDelay
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	return d
}

// retryableStatus reports whether a non-2xx response is worth retrying
// (transient) rather than returned immediately - a real validation or
// auth failure will fail identically on every attempt, so retrying it
// only adds latency.
func (p retryPolicy) retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
