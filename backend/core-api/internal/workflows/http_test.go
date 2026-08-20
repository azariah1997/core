package workflows_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestStartHandlerRejectsMissingType(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	mux := http.NewServeMux()
	workflows.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/workflows", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStartAndGetWorkflowRoundTripThroughTheRealRouter(t *testing.T) {
	temporal := newFakeTemporal()
	temporal.execution = workflows.Execution{Status: workflows.StatusCompleted, Result: map[string]any{"outcome": "approved"}}
	svc := newService(temporal, nil)
	mux := http.NewServeMux()
	workflows.RegisterRoutes(mux, svc, fixedUser("u1"))

	startReq := httptest.NewRequest("POST", "/v1/workflows", strings.NewReader(`{"type":"approval"}`))
	startRR := httptest.NewRecorder()
	mux.ServeHTTP(startRR, startReq)
	if startRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", startRR.Code, startRR.Body.String())
	}
	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(startRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getReq := httptest.NewRequest("GET", "/v1/workflows/"+body.WorkflowID, nil)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `"outcome":"approved"`) {
		t.Fatalf("expected the live execution result in the response, got %s", getRR.Body.String())
	}
}

func TestGetHandlerForbidsAStrangerFromReadingAWorkflow(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	ownerMux := http.NewServeMux()
	workflows.RegisterRoutes(ownerMux, svc, fixedUser("owner"))

	startRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(startRR, httptest.NewRequest("POST", "/v1/workflows", strings.NewReader(`{"type":"approval"}`)))
	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(startRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	strangerMux := http.NewServeMux()
	workflows.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))
	getReq := httptest.NewRequest("GET", "/v1/workflows/"+body.WorkflowID, nil)
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, getReq)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSignalHandlerRejectsMissingName(t *testing.T) {
	svc := newService(newFakeTemporal(), nil)
	mux := http.NewServeMux()
	workflows.RegisterRoutes(mux, svc, fixedUser("u1"))

	startRR := httptest.NewRecorder()
	mux.ServeHTTP(startRR, httptest.NewRequest("POST", "/v1/workflows", strings.NewReader(`{"type":"approval"}`)))
	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.Unmarshal(startRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	signalReq := httptest.NewRequest("POST", "/v1/workflows/"+body.WorkflowID+"/signal", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, signalReq)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
