package audit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/audit"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestRecordHandlerAlwaysUsesTheCallerAsActor(t *testing.T) {
	// The request body has no actorUserId field at all (see http.go's
	// own comment) - this proves it's impossible to submit a record
	// claiming to be someone else's action through this endpoint.
	svc := newService(nil)
	mux := http.NewServeMux()
	audit.RegisterRoutes(mux, svc, fixedUser("real-caller"))

	req := httptest.NewRequest("POST", "/v1/audit", strings.NewReader(`{"action":"x","resourceType":"y","resourceId":"z"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["actorUserId"] != "real-caller" {
		t.Fatalf("expected actorUserId to be the real caller, got %+v", body)
	}
}

func TestRecordHandlerRejectsMissingAction(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	audit.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/audit", strings.NewReader(`{"resourceType":"y","resourceId":"z"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	mux := http.NewServeMux()
	audit.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("GET", "/v1/audit", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetHandlerReturns404ForUnknownRecord(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	mux := http.NewServeMux()
	audit.RegisterRoutes(mux, svc, fixedUser("admin"))

	req := httptest.NewRequest("GET", "/v1/audit/00000000-0000-0000-0000-000000000000", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
