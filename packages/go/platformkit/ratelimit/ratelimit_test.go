package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/packages/go/platformkit/ratelimit"
)

func newLimiter(t *testing.T) (*ratelimit.Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return ratelimit.New(client), mr
}

func TestAllowUnderLimit(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ok, err := l.Allow(ctx, "k1", 3, time.Minute)
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !ok {
			t.Fatalf("expected call %d to be allowed under a limit of 3", i+1)
		}
	}
}

func TestDeniesOverLimit(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := l.Allow(ctx, "k1", 3, time.Minute); err != nil {
			t.Fatalf("allow: %v", err)
		}
	}
	ok, err := l.Allow(ctx, "k1", 3, time.Minute)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if ok {
		t.Fatal("expected the 4th call to be denied under a limit of 3")
	}
}

func TestDifferentKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := l.Allow(ctx, "k1", 3, time.Minute); err != nil {
			t.Fatalf("allow k1: %v", err)
		}
	}
	ok, err := l.Allow(ctx, "k2", 3, time.Minute)
	if err != nil {
		t.Fatalf("allow k2: %v", err)
	}
	if !ok {
		t.Fatal("expected a different key to have its own independent count")
	}
}

func TestWindowResetsTheCount(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := l.Allow(ctx, "k1", 3, time.Minute); err != nil {
			t.Fatalf("allow: %v", err)
		}
	}
	if ok, _ := l.Allow(ctx, "k1", 3, time.Minute); ok {
		t.Fatal("expected the count to be exhausted before fast-forwarding")
	}
	mr.FastForward(time.Minute + time.Second)
	ok, err := l.Allow(ctx, "k1", 3, time.Minute)
	if err != nil {
		t.Fatalf("allow after window: %v", err)
	}
	if !ok {
		t.Fatal("expected the window to have reset after it elapsed")
	}
}
