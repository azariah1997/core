package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/realtime-gateway/internal/presence"
	"github.com/example/core-platform/backend/realtime-gateway/internal/ws"
)

func newTestTracker(t *testing.T) *presence.Tracker {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return presence.New(client, 45*time.Second)
}

func withFixedAuth(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(ws.WithAuth(r.Context(), ws.Auth{UserID: userID})))
		})
	}
}

func TestPresenceHandlerReportsOfflineForAUserWithNoConnection(t *testing.T) {
	tracker := newTestTracker(t)
	mux := http.NewServeMux()
	mux.Handle("GET /v1/presence/{userId}", withFixedAuth("caller-1")(presenceHandler(tracker)))

	req := httptest.NewRequest("GET", "/v1/presence/user-2", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		UserID string `json:"userId"`
		Online bool   `json:"online"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Online {
		t.Fatalf("expected offline for a user with no presence record, got %+v", body)
	}
}

func TestPresenceHandlerReportsOnlineAfterARealConnect(t *testing.T) {
	tracker := newTestTracker(t)
	if err := tracker.Connect(context.Background(), "user-2", "device-1", "conn-1"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /v1/presence/{userId}", withFixedAuth("caller-1")(presenceHandler(tracker)))

	req := httptest.NewRequest("GET", "/v1/presence/user-2", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Online bool `json:"online"`
	}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if !body.Online {
		t.Fatal("expected online after a real presence Connect")
	}
}
