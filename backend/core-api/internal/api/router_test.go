package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/example/core-platform/packages/go/platformkit/config"
)

func TestLiveness(t *testing.T) {
	req := httptest.NewRequest("GET", "/livez", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGenericDataAPIIsBlocked(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/data/query", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 501 {
		t.Fatalf("expected 501, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "NOT_IMPLEMENTED" {
		t.Fatalf("expected standard error envelope with code NOT_IMPLEMENTED, got %v", body)
	}
}

func TestUnmatchedRouteReturnsStandardNotFoundEnvelope(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/does-not-exist", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "RESOURCE_NOT_FOUND" {
		t.Fatalf("expected standard error envelope with code RESOURCE_NOT_FOUND, got %v", body)
	}
}

func TestCorrelationIDIsReflectedOnResponse(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/platform", nil)
	req.Header.Set("X-Correlation-ID", "test-correlation-id")
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Correlation-ID"); got != "test-correlation-id" {
		t.Fatalf("expected correlation ID to be echoed back, got %q", got)
	}
}
