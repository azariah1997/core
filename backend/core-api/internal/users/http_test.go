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

// fakeAccess is a stand-in for the api package's real authz-backed
// AccessChecker - these tests exercise the HTTP handlers, not
// authorization policy itself (that's tested where the real policy lives,
// in internal/api).
type fakeAccess struct{ allow bool }

func (f fakeAccess) CanViewProfile(ctx context.Context, subjectUserID, targetUserID string) (bool, error) {
	return f.allow, nil
}

func (f fakeAccess) IsPlatformAdmin(ctx context.Context, subjectUserID string) (bool, error) {
	return f.allow, nil
}

func TestMeReturnsAttachedUser(t *testing.T) {
	svc := newService()
	u, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Ada"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(u), fakeAccess{allow: true})

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
	users.RegisterRoutes(mux, svc, fixedUser(u), fakeAccess{allow: true})

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
	caller, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Caller"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(caller), fakeAccess{allow: true})

	req := httptest.NewRequest("GET", "/v1/users/11111111-1111-1111-1111-111111111111", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetByIDReturnsExistingUser(t *testing.T) {
	svc := newService()
	caller, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Caller"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	target, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Target"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(caller), fakeAccess{allow: true})

	req := httptest.NewRequest("GET", "/v1/users/"+target.ID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetByIDDeniedByAccessCheckerReturns403(t *testing.T) {
	svc := newService()
	caller, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Caller"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	target, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Target"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(caller), fakeAccess{allow: false})

	req := httptest.NewRequest("GET", "/v1/users/"+target.ID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListRequiresPlatformAdmin(t *testing.T) {
	svc := newService()
	caller, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Caller"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(caller), fakeAccess{allow: false})

	req := httptest.NewRequest("GET", "/v1/users", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-admin, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListReturnsUsersForAnAdmin(t *testing.T) {
	svc := newService()
	admin, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Admin"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := svc.Create(context.Background(), users.CreateInput{DisplayName: "Someone Else"}); err != nil {
		t.Fatalf("create other: %v", err)
	}

	mux := http.NewServeMux()
	users.RegisterRoutes(mux, svc, fixedUser(admin), fakeAccess{allow: true})

	req := httptest.NewRequest("GET", "/v1/users", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 users, got %d: %+v", len(body.Items), body.Items)
	}
}
