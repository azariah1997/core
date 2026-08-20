package analytics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/analytics"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestTrackHandlerAcceptsARealRequestWithNoAuthorizationHeaderAtAll(t *testing.T) {
	// RegisterTrackRoute deliberately wires no auth middleware at all -
	// this is this platform's one open write endpoint (see
	// analytics/README.md) - so the real test is that a bare mux.ServeHTTP
	// with zero context setup still succeeds.
	svc := newService(nil, true)
	mux := http.NewServeMux()
	analytics.RegisterTrackRoute(mux, svc)

	body := `{"events":[{"eventName":"page_view","anonymousId":"anon-1","appId":"app1"}]}`
	req := httptest.NewRequest("POST", "/v1/analytics/events", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"accepted":1`) {
		t.Fatalf("expected accepted:1, got %s", rr.Body.String())
	}
}

func TestTrackHandlerRejectsInvalidJSON(t *testing.T) {
	svc := newService(nil, true)
	mux := http.NewServeMux()
	analytics.RegisterTrackRoute(mux, svc)

	req := httptest.NewRequest("POST", "/v1/analytics/events", strings.NewReader(`not json`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListRecentHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	mux := http.NewServeMux()
	analytics.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("GET", "/v1/analytics/events", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListRecentHandlerReturnsEventsForAnAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, true)
	trackMux := http.NewServeMux()
	analytics.RegisterTrackRoute(trackMux, svc)
	trackMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/analytics/events",
		strings.NewReader(`{"events":[{"eventName":"page_view","anonymousId":"anon-1","appId":"app1"}]}`)))

	mux := http.NewServeMux()
	analytics.RegisterRoutes(mux, svc, fixedUser("admin"))
	req := httptest.NewRequest("GET", "/v1/analytics/events", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "page_view") {
		t.Fatalf("expected the tracked event in the response, got %s", rr.Body.String())
	}
}
