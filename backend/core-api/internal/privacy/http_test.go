package privacy_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/privacy"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestSetConsentHandlerRejectsMissingPurpose(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	mux := http.NewServeMux()
	privacy.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/privacy/consent", strings.NewReader(`{"granted":true}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetAndListConsentRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	mux := http.NewServeMux()
	privacy.RegisterRoutes(mux, svc, fixedUser("u1"))

	setReq := httptest.NewRequest("POST", "/v1/privacy/consent", strings.NewReader(`{"purpose":"marketing","granted":true,"version":"v1"}`))
	setRR := httptest.NewRecorder()
	mux.ServeHTTP(setRR, setReq)
	if setRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", setRR.Code, setRR.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/v1/privacy/consent", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "marketing") {
		t.Fatalf("expected the recorded consent in the response, got %s", listRR.Body.String())
	}
}

func TestCreateRetentionPolicyHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, &fakeStarter{}, newFakeStore())
	mux := http.NewServeMux()
	privacy.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("POST", "/v1/privacy/retention-policies", strings.NewReader(`{"resourceType":"message","retentionDays":30}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRequestExportHandlerReturnsAcceptedAndTracksStatus(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	mux := http.NewServeMux()
	privacy.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/privacy/export", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"running"`) {
		t.Fatalf("expected a running export request, got %s", rr.Body.String())
	}
}

func TestGetExportHandlerForbidsAStrangerFromReadingAnotherUsersExport(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	ownerMux := http.NewServeMux()
	privacy.RegisterRoutes(ownerMux, svc, fixedUser("owner"))

	createRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/privacy/export", nil))
	if createRR.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	strangerMux := http.NewServeMux()
	privacy.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))
	getReq := httptest.NewRequest("GET", "/v1/privacy/export/"+body.ID, nil)
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, getReq)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}
