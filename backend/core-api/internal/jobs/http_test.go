package jobs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/jobs"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestEnqueueHandlerRejectsMissingType(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	jobs.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/jobs", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEnqueueAndGetJobRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	jobs.RegisterRoutes(mux, svc, fixedUser("u1"))

	enqueueReq := httptest.NewRequest("POST", "/v1/jobs", strings.NewReader(`{"type":"echo","payload":{"msg":"hi"}}`))
	enqueueRR := httptest.NewRecorder()
	mux.ServeHTTP(enqueueRR, enqueueReq)
	if enqueueRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", enqueueRR.Code, enqueueRR.Body.String())
	}

	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(enqueueRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getReq := httptest.NewRequest("GET", "/v1/jobs/"+body.ID, nil)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `"type":"echo"`) {
		t.Fatalf("expected the enqueued job in the response, got %s", getRR.Body.String())
	}
}

func TestGetHandlerForbidsAStrangerFromReadingAJob(t *testing.T) {
	svc := newService(nil)
	ownerMux := http.NewServeMux()
	jobs.RegisterRoutes(ownerMux, svc, fixedUser("owner"))

	enqueueReq := httptest.NewRequest("POST", "/v1/jobs", strings.NewReader(`{"type":"echo"}`))
	enqueueRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(enqueueRR, enqueueReq)
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(enqueueRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	strangerMux := http.NewServeMux()
	jobs.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))
	getReq := httptest.NewRequest("GET", "/v1/jobs/"+body.ID, nil)
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, getReq)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListMineHandlerReturnsOnlyOwnJobs(t *testing.T) {
	svc := newService(nil)
	u1Mux := http.NewServeMux()
	jobs.RegisterRoutes(u1Mux, svc, fixedUser("u1"))
	u1Mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs", strings.NewReader(`{"type":"echo"}`)))

	u2Mux := http.NewServeMux()
	jobs.RegisterRoutes(u2Mux, svc, fixedUser("u2"))
	u2Mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/jobs", strings.NewReader(`{"type":"echo"}`)))

	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	rr := httptest.NewRecorder()
	u1Mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Count(rr.Body.String(), `"type":"echo"`) != 1 {
		t.Fatalf("expected exactly u1's own job, got %s", rr.Body.String())
	}
}
