package applications_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	"github.com/example/core-platform/backend/core-api/internal/applications/memory"
)

func newHandler() http.Handler {
	mux := http.NewServeMux()
	applications.RegisterRoutes(mux, applications.NewService(memory.New()))
	return mux
}

func TestPostCreatesApplication(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["slug"] != "demo" || body["status"] != "active" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestPostRejectsMalformedJSON(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`not json`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestGetUnknownApplicationReturns404Envelope(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("GET", "/v1/apps/11111111-1111-1111-1111-111111111111", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["code"] != "RESOURCE_NOT_FOUND" {
		t.Fatalf("expected RESOURCE_NOT_FOUND envelope, got %v", body)
	}
}

func TestDuplicateSlugReturns409(t *testing.T) {
	h := newHandler()
	first := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo"}`))
	h.ServeHTTP(httptest.NewRecorder(), first)

	second := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo 2"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, second)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListReturnsItemsEnvelope(t *testing.T) {
	h := newHandler()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo"}`)))

	req := httptest.NewRequest("GET", "/v1/apps", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
}

func TestPatchUpdatesApplication(t *testing.T) {
	h := newHandler()
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo"}`)))
	var created map[string]any
	_ = json.NewDecoder(createRR.Body).Decode(&created)
	id := created["id"].(string)

	req := httptest.NewRequest("PATCH", "/v1/apps/"+id, strings.NewReader(`{"status":"inactive"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&updated)
	if updated["status"] != "inactive" {
		t.Fatalf("expected status inactive, got %v", updated["status"])
	}
}

func TestPatchWithNoFieldsReturnsValidationError(t *testing.T) {
	h := newHandler()
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo","name":"Demo"}`)))
	var created map[string]any
	_ = json.NewDecoder(createRR.Body).Decode(&created)
	id := created["id"].(string)

	req := httptest.NewRequest("PATCH", "/v1/apps/"+id, strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
