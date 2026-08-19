package coresdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDoAttachesBearerTokenAndCorrelationID(t *testing.T) {
	var gotAuth, gotCorrelation string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCorrelation = r.Header.Get(correlationIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, WithTokenSource(StaticTokenSource("test-token")))
	if err := client.Do(context.Background(), "GET", "/whatever", nil, nil); err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected Bearer test-token, got %q", gotAuth)
	}
	if gotCorrelation == "" {
		t.Fatal("expected a non-empty correlation ID to be generated")
	}
}

func TestDoPropagatesExplicitCorrelationID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(correlationIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.Do(context.Background(), "GET", "/x", nil, nil, WithCorrelationID("caller-supplied-id"))
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if got != "caller-supplied-id" {
		t.Fatalf("expected caller-supplied-id, got %q", got)
	}
}

func TestDoDecodesSuccessResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"name": "core-platform"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	var out struct {
		Name string `json:"name"`
	}
	if err := client.Do(context.Background(), "GET", "/x", nil, &out); err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if out.Name != "core-platform" {
		t.Fatalf("expected core-platform, got %q", out.Name)
	}
}

func TestDoDecodesRealErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"code":          "ACCESS_DENIED",
			"message":       "not allowed",
			"correlationId": "abc-123",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsCode(err, CodeAccessDenied) {
		t.Fatalf("expected ACCESS_DENIED, got %v", err)
	}
	apiErr := err.(*APIError)
	if apiErr.StatusCode != http.StatusForbidden || apiErr.CorrelationID != "abc-123" {
		t.Fatalf("unexpected APIError fields: %+v", apiErr)
	}
}

func TestDoSendsJSONRequestBody(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %q", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.Do(context.Background(), "POST", "/x", map[string]string{"name": "acme"}, nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if got["name"] != "acme" {
		t.Fatalf("expected request body to carry name=acme, got %v", got)
	}
}

func TestDoRetriesGETOnTransientStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"code": "DEPENDENCY_FAILURE", "message": "try again"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, WithRetries(3, 0))
	err := client.Do(context.Background(), "GET", "/x", nil, nil)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestDoDoesNotRetryPOSTByDefault(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"code": "DEPENDENCY_FAILURE", "message": "down"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, WithRetries(3, 0))
	err := client.Do(context.Background(), "POST", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt for POST (retries where safe means GET-only by default), got %d", calls)
	}
}

func TestDoDoesNotRetryOnNonTransientStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"code": "RESOURCE_NOT_FOUND", "message": "missing"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, WithRetries(3, 0))
	err := client.Do(context.Background(), "GET", "/x", nil, nil)
	if !IsCode(err, CodeNotFound) {
		t.Fatalf("expected RESOURCE_NOT_FOUND, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt (404 is not transient), got %d", calls)
	}
}
