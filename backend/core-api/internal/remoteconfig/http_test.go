package remoteconfig_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestSetHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	remoteconfig.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("PUT", "/v1/config/app-1/production/x", strings.NewReader(`{"value":1}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetAndGetRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	adminMux := http.NewServeMux()
	remoteconfig.RegisterRoutes(adminMux, svc, fixedUser("admin"))

	setReq := httptest.NewRequest("PUT", "/v1/config/app-1/production/checkout.maxRetries", strings.NewReader(`{"value":3}`))
	setRR := httptest.NewRecorder()
	adminMux.ServeHTTP(setRR, setReq)
	if setRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", setRR.Code, setRR.Body.String())
	}

	readerMux := http.NewServeMux()
	remoteconfig.RegisterRoutes(readerMux, svc, fixedUser("any-authenticated-user"))
	getReq := httptest.NewRequest("GET", "/v1/config/app-1/production/checkout.maxRetries", nil)
	getRR := httptest.NewRecorder()
	readerMux.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `"value":3`) {
		t.Fatalf("expected the set value in the response, got %s", getRR.Body.String())
	}
}

func TestListHandlerRequiresAppIDAndEnvironmentQueryParameters(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	remoteconfig.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/config", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHistoryHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	adminMux := http.NewServeMux()
	remoteconfig.RegisterRoutes(adminMux, svc, fixedUser("admin"))
	adminMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/v1/config/app-1/production/x", strings.NewReader(`{"value":1}`)))

	mux := http.NewServeMux()
	remoteconfig.RegisterRoutes(mux, svc, fixedUser("not-admin"))
	req := httptest.NewRequest("GET", "/v1/config/app-1/production/x/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}
