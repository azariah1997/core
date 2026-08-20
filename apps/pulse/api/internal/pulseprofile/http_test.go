package pulseprofile_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprofile"
)

// fixedCaller mirrors Core's own fixedUser test fixture (see e.g.
// backend/core-api/internal/users/http_test.go) - the same
// attach-to-context-directly pattern, just against pulseauth's context
// key instead of Core's, since pulseauth.RequireUser's real network
// call to core-api isn't what these handler tests are proving.
func fixedCaller(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(pulseauth.WithCaller(r.Context(), id)))
		})
	}
}

func TestEnsureProfileHandlerRejectsAnInvalidHandle(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	pulseprofile.RegisterRoutes(mux, svc, fixedCaller("user-1"))

	req := httptest.NewRequest("POST", "/v1/pulse/profile", strings.NewReader(`{"handle":"a"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEnsureAndGetProfileRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	pulseprofile.RegisterRoutes(mux, svc, fixedCaller("user-1"))

	createReq := httptest.NewRequest("POST", "/v1/pulse/profile", strings.NewReader(`{"handle":"rachel"}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	getReq := httptest.NewRequest("GET", "/v1/pulse/profile/me", nil)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `"handle":"rachel"`) {
		t.Fatalf("expected the created profile in the response, got %s", getRR.Body.String())
	}
}

func TestGetByHandleHandlerReturns404ForAnUnknownHandle(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	pulseprofile.RegisterRoutes(mux, svc, fixedCaller("user-1"))

	req := httptest.NewRequest("GET", "/v1/pulse/profile/does-not-exist", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateProfileHandlerRoundTripsPreferences(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	pulseprofile.RegisterRoutes(mux, svc, fixedCaller("user-1"))

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/pulse/profile", strings.NewReader(`{"handle":"rachel"}`)))

	updateReq := httptest.NewRequest("PATCH", "/v1/pulse/profile/me", strings.NewReader(`{"visualPrefs":{"theme":"dark"}}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, updateReq)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"theme":"dark"`) {
		t.Fatalf("expected the updated preference in the response, got %s", rr.Body.String())
	}
}
