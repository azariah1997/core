// Package ratelimit is a small, shared Redis/Valkey-backed rate limiter -
// "rate limiting" and "spam protection" are two of Phase 21's named
// requirements, and cross-cutting infrastructure any service might need,
// not something specific to Trust & Safety. It lives in platformkit
// alongside rtbus and searchidx for the same reason those do: one real
// implementation every caller shares, rather than each module hand-
// rolling a compatible-by-luck counter.
//
// The algorithm is a plain fixed-window counter (INCR + EXPIRE), not a
// sliding window or token bucket - honest about its own limitation (a
// caller can burst up to 2x the limit across a window boundary) rather
// than implementing more precision than this platform's first real
// consumer (Phase 21's report-spam protection) actually needs.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	redis *redis.Client
}

func New(client *redis.Client) *Limiter {
	return &Limiter{redis: client}
}

// Allow reports whether one more action under key is permitted within
// limit occurrences per window, incrementing the count as a side effect
// of asking - the same "checking counts as consuming" contract as any
// fixed-window limiter. The counter's own key expires after window, so a
// caller never needs a separate cleanup path.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	count, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("ratelimit incr: %w", err)
	}
	if count == 1 {
		if err := l.redis.Expire(ctx, key, window).Err(); err != nil {
			return false, fmt.Errorf("ratelimit expire: %w", err)
		}
	}
	return count <= int64(limit), nil
}
