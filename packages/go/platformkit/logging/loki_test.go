package logging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestNewWithLokiPushesARealStreamEntry(t *testing.T) {
	var mu sync.Mutex
	var received lokiPushRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("expected POST to /loki/api/v1/push, got %s", r.URL.Path)
		}
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode push body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	logger := NewWithLoki("test-service", "test-env", srv.URL+"/loki/api/v1/push")
	logger.Info("hello from a test", "orderId", "abc-123")

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received.Streams) > 0
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(received.Streams))
	}
	stream := received.Streams[0]
	if stream.Stream["service"] != "test-service" || stream.Stream["env"] != "test-env" || stream.Stream["level"] != "INFO" {
		t.Fatalf("unexpected stream labels: %+v", stream.Stream)
	}
	if len(stream.Values) != 1 {
		t.Fatalf("expected 1 log value, got %d", len(stream.Values))
	}

	var line map[string]any
	if err := json.Unmarshal([]byte(stream.Values[0][1]), &line); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if line["msg"] != "hello from a test" {
		t.Fatalf("expected msg to be preserved, got %+v", line)
	}
	if line["orderId"] != "abc-123" {
		t.Fatalf("expected structured attrs to be preserved, got %+v", line)
	}
}

func TestNewWithLokiAttachesRealTraceContext(t *testing.T) {
	var mu sync.Mutex
	var received lokiPushRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	logger := NewWithLoki("test-service", "test-env", srv.URL+"/loki/api/v1/push")

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	logger.InfoContext(ctx, "a request within a real span")

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received.Streams) > 0
	})

	mu.Lock()
	defer mu.Unlock()
	var line map[string]any
	json.Unmarshal([]byte(received.Streams[0].Values[0][1]), &line)
	if line["traceId"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected the real trace ID to be attached, got %+v", line)
	}
	if line["spanId"] != "00f067aa0ba902b7" {
		t.Fatalf("expected the real span ID to be attached, got %+v", line)
	}
}

func TestNewWithLokiStillLogsToStdoutOnPushFailure(t *testing.T) {
	// An unreachable Loki must never prevent the underlying stdout
	// handler from doing its job - logging is fire-and-forget to Loki.
	logger := NewWithLoki("test-service", "test-env", "http://127.0.0.1:1/loki/api/v1/push")
	logger.Info("this must not panic or block indefinitely")
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
