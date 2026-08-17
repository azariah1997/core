package users_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/users"
)

// fixedUser is a stand-in for the api package's real requireUser middleware
// (which resolves a caller from an authenticated identity) - here we just
// attach a known user directly, so these tests exercise the HTTP handlers
// in isolation from identity composition.
func fixedUser(u users.User) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), u)))
		})
	}
}

func passthrough(next http.Handler) http.Handler { return next }

func TestMeReturnsAttachedUser(t *testing.T) {
	svc := newService()
	u, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(u), passthrough)

	req := httptest.NewRequest("GET", "/v1/users/me", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["displayName"] != "Ada" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestPatchMeUpdatesDisplayName(t *testing.T) {
	svc := newService()
	u, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(u), passthrough)

	req := httptest.NewRequest("PATCH", "/v1/users/me", strings.NewReader(`{"displayName":"Ada Lovelace"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["displayName"] != "Ada Lovelace" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestGetByIDReturns404ForUnknownUser(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, passthrough, passthrough)

	req := httptest.NewRequest("GET", "/v1/users/11111111-1111-1111-1111-111111111111", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetByIDReturnsExistingUser(t *testing.T) {
	svc := newService()
	u, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, passthrough, passthrough)

	req := httptest.NewRequest("GET", "/v1/users/"+u.ID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
