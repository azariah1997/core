package jobrunner

import "time"

const maxBackoff = 5 * time.Minute

// backoffFor is a plain exponential backoff (2^attempt seconds, capped)
// with no jitter - the roadmap asks for "retry," not a specific backoff
// curve, and this is the simplest correct one. attempt is the attempt
// number that just failed (1-indexed), so the first retry waits 2s, the
// second 4s, and so on up to the cap.
func backoffFor(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 20 { // guard against overflow in the shift below
		return maxBackoff
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
