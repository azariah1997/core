package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/core-platform/backend/worker/internal/jobrunner/handlers"
)

func TestEchoAlwaysSucceeds(t *testing.T) {
	echo := handlers.Echo(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := echo(context.Background(), map[string]any{"hello": "world"}); err != nil {
		t.Fatalf("expected echo to always succeed, got %v", err)
	}
}

func TestWebhookDeliversToARealServer(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	webhook := handlers.Webhook(nil)
	err := webhook(context.Background(), map[string]any{
		"url": srv.URL, "body": map[string]any{"event": "test"},
	})
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	if receivedBody["event"] != "test" {
		t.Fatalf("expected the server to receive the body, got %v", receivedBody)
	}
}

func TestWebhookFailsOnNon2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	webhook := handlers.Webhook(nil)
	err := webhook(context.Background(), map[string]any{"url": srv.URL})
	if err == nil {
		t.Fatal("expected a non-2xx response to be a real failure")
	}
}

func TestWebhookFailsOnConnectionRefused(t *testing.T) {
	webhook := handlers.Webhook(nil)
	// Nothing listens here - a genuine connection-refused failure, not simulated.
	err := webhook(context.Background(), map[string]any{"url": "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("expected a connection failure to a dead port to be a real failure")
	}
}

func TestWebhookRequiresURL(t *testing.T) {
	webhook := handlers.Webhook(nil)
	if err := webhook(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected a missing url to fail validation")
	}
}
