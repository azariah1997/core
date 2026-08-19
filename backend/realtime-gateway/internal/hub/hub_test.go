package hub_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/realtime-gateway/internal/hub"
	"github.com/example/core-platform/packages/go/platformkit/metrics"
)

// realtimeConnectionsGauge reads the real, current value of
// realtime_ws_connections off metrics.Handler()'s actual exposition
// output - the same package other services scrape in production,
// rather than a second, parallel accessor that only exists for tests.
func realtimeConnectionsGauge(t *testing.T) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	// (?m)^...$ anchors to the actual metric line, not the "# HELP
	// realtime_ws_connections Currently active..." comment line above it.
	m := regexp.MustCompile(`(?m)^realtime_ws_connections (\S+)$`).FindSubmatch(body)
	if m == nil {
		t.Fatal("realtime_ws_connections not found in /metrics output")
	}
	v, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		t.Fatalf("parse gauge value: %v", err)
	}
	return v
}

func newHub(t *testing.T) *hub.Hub {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	h := hub.New(client, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.Run(ctx)
	// Give the PSubscribe goroutine a moment to actually subscribe before
	// the test starts publishing - otherwise an early publish can race
	// ahead of the subscription and never be delivered.
	time.Sleep(50 * time.Millisecond)
	return h
}

func recv(t *testing.T, ch <-chan []byte) map[string]any {
	t.Helper()
	select {
	case payload := <-ch:
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

func TestPublishToChannelDeliversToSubscribers(t *testing.T) {
	h := newHub(t)
	a := &hub.Conn{ID: "a", UserID: "u1", DeviceID: "d1", Send: make(chan []byte, 4)}
	b := &hub.Conn{ID: "b", UserID: "u2", DeviceID: "d1", Send: make(chan []byte, 4)}
	h.Register(a)
	h.Register(b)

	h.Subscribe(a.ID, "room:1")
	// b never subscribes - should not receive anything.

	if err := h.PublishToChannel(context.Background(), "room:1", json.RawMessage(`{"hello":"world"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	msg := recv(t, a.Send)
	if msg["hello"] != "world" {
		t.Fatalf("unexpected message: %v", msg)
	}

	select {
	case payload := <-b.Send:
		t.Fatalf("expected no delivery to non-subscriber, got %s", payload)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := newHub(t)
	a := &hub.Conn{ID: "a", UserID: "u1", DeviceID: "d1", Send: make(chan []byte, 4)}
	h.Register(a)
	h.Subscribe(a.ID, "room:1")
	h.Unsubscribe(a.ID, "room:1")

	if err := h.PublishToChannel(context.Background(), "room:1", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case payload := <-a.Send:
		t.Fatalf("expected no delivery after unsubscribe, got %s", payload)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPublishToUserDeliversToAllOfThatUsersDevices(t *testing.T) {
	h := newHub(t)
	phone := &hub.Conn{ID: "phone", UserID: "u1", DeviceID: "phone", Send: make(chan []byte, 4)}
	laptop := &hub.Conn{ID: "laptop", UserID: "u1", DeviceID: "laptop", Send: make(chan []byte, 4)}
	other := &hub.Conn{ID: "other", UserID: "u2", DeviceID: "phone", Send: make(chan []byte, 4)}
	h.Register(phone)
	h.Register(laptop)
	h.Register(other)

	if err := h.PublishToUser(context.Background(), "u1", json.RawMessage(`{"n":1}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recv(t, phone.Send)
	recv(t, laptop.Send)
	select {
	case payload := <-other.Send:
		t.Fatalf("expected u2 to receive nothing, got %s", payload)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPublishToDeviceDeliversToExactlyOneDevice(t *testing.T) {
	h := newHub(t)
	phone := &hub.Conn{ID: "phone", UserID: "u1", DeviceID: "phone", Send: make(chan []byte, 4)}
	laptop := &hub.Conn{ID: "laptop", UserID: "u1", DeviceID: "laptop", Send: make(chan []byte, 4)}
	h.Register(phone)
	h.Register(laptop)

	if err := h.PublishToDevice(context.Background(), "u1", "laptop", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recv(t, laptop.Send)
	select {
	case payload := <-phone.Send:
		t.Fatalf("expected phone to receive nothing, got %s", payload)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestRealtimeConnectionsGaugeStaysAccurateAcrossRegisterEvictUnregister(t *testing.T) {
	// Delta-based, not an absolute assertion - realtime_ws_connections is
	// process-wide shared state (promauto's usual singleton pattern), so
	// other tests in this same binary may have already nudged it.
	h := newHub(t)
	before := realtimeConnectionsGauge(t)

	first := &hub.Conn{ID: "gauge-first", UserID: "gauge-u1", DeviceID: "gauge-d1", Send: make(chan []byte, 4)}
	h.Register(first)
	if got, want := realtimeConnectionsGauge(t), before+1; got != want {
		t.Fatalf("after one Register: got %v, want %v", got, want)
	}

	// A same-user-same-device reconnect evicts the old connection and
	// registers a new one - net zero change, not +1 (which would mean
	// the gauge silently drifts upward on every reconnect).
	second := &hub.Conn{ID: "gauge-second", UserID: "gauge-u1", DeviceID: "gauge-d1", Send: make(chan []byte, 4)}
	h.Register(second)
	if got, want := realtimeConnectionsGauge(t), before+1; got != want {
		t.Fatalf("after the evicting Register: got %v, want %v (eviction must be net zero)", got, want)
	}

	// The evicted connection's own goroutine calling Unregister afterward
	// (as it really would, once it notices Send is closed) must not
	// double-decrement - Register already accounted for the eviction.
	h.Unregister(first)
	if got, want := realtimeConnectionsGauge(t), before+1; got != want {
		t.Fatalf("after Unregister(evicted first): got %v, want %v (must not double-decrement)", got, want)
	}

	h.Unregister(second)
	if got, want := realtimeConnectionsGauge(t), before; got != want {
		t.Fatalf("after Unregister(second): got %v, want %v", got, want)
	}
}

func TestRegisterEvictsExistingConnectionForSameUserDevice(t *testing.T) {
	h := newHub(t)
	first := &hub.Conn{ID: "first", UserID: "u1", DeviceID: "d1", Send: make(chan []byte, 4)}
	h.Register(first)

	second := &hub.Conn{ID: "second", UserID: "u1", DeviceID: "d1", Send: make(chan []byte, 4)}
	h.Register(second)

	// The old connection's Send channel should be closed so its write
	// loop notices and tears the socket down.
	select {
	case _, ok := <-first.Send:
		if ok {
			t.Fatal("expected first.Send to be closed, got a value instead")
		}
	case <-time.After(time.Second):
		t.Fatal("expected first.Send to be closed promptly")
	}

	if err := h.PublishToDevice(context.Background(), "u1", "d1", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	recv(t, second.Send) // only the surviving connection should get it
}

func TestUnregisterRemovesSubscriptions(t *testing.T) {
	h := newHub(t)
	a := &hub.Conn{ID: "a", UserID: "u1", DeviceID: "d1", Send: make(chan []byte, 4)}
	h.Register(a)
	h.Subscribe(a.ID, "room:1")
	if !h.IsSubscribed(a.ID, "room:1") {
		t.Fatal("expected a to be subscribed")
	}

	h.Unregister(a)
	if h.IsSubscribed(a.ID, "room:1") {
		t.Fatal("expected subscription to be cleared on unregister")
	}
}
