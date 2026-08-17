package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationsmemory "github.com/example/core-platform/backend/core-api/internal/applications/memory"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	identitymemory "github.com/example/core-platform/backend/core-api/internal/identity/memory"
	"github.com/example/core-platform/packages/go/platformkit/config"
)

func newTestHandler() http.Handler {
	return New(
		config.Load(),
		applications.NewService(applicationsmemory.New()),
		identity.NewService("fake", identitymemory.Provider{}, identitymemory.New()),
	)
}

func TestLiveness(t *testing.T) {
	req := httptest.NewRequest("GET", "/livez", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGenericDataAPIIsBlocked(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/data/query", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
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
	newTestHandler().ServeHTTP(rr, req)
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
	newTestHandler().ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Correlation-ID"); got != "test-correlation-id" {
		t.Fatalf("expected correlation ID to be echoed back, got %q", got)
	}
}

func TestCreateAndGetApplicationEndToEnd(t *testing.T) {
	handler := newTestHandler()

	createReq := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo-app","name":"Demo App"}`))
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)
	if createRR.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("expected created application to have an id")
	}

	getReq := httptest.NewRequest("GET", "/v1/apps/"+id, nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
}

func TestCreateApplicationRejectsInvalidSlug(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"Not Valid!","name":"x"}`))
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
