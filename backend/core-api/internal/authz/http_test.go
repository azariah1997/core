package authz_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestListRolesHandlerDefaultsToTheCallersOwnRoles(t *testing.T) {
	svc := newService()
	if err := svc.AssignRole(context.Background(), "u1", "u1", authz.RoleSupport); err != nil {
		t.Fatalf("bootstrap assign: %v", err)
	}

	mux := http.NewServeMux()
	authz.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/authz/roles", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "support") {
		t.Fatalf("expected the caller's own role in the response, got %s", rr.Body.String())
	}
}

func TestListRolesHandlerForbidsViewingAStrangersRoles(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	authz.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("GET", "/v1/authz/roles?userId=someone-else", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAssignRoleHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	authz.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("POST", "/v1/authz/roles", strings.NewReader(`{"userId":"target","role":"support"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAssignAndRevokeRoleHandlersRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService()
	// Bootstrap: grant "admin" platform.admin directly at the service
	// layer, the same real gap Phase 25's authz HTTP surface exists to
	// close for everyone *except* the very first admin (see
	// docs/RUNBOOKS.md's "Bootstrap the first platform.admin").
	if err := svc.AssignRole(context.Background(), "admin", "admin", authz.RolePlatformAdmin); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	mux := http.NewServeMux()
	authz.RegisterRoutes(mux, svc, fixedUser("admin"))

	assignReq := httptest.NewRequest("POST", "/v1/authz/roles", strings.NewReader(`{"userId":"target","role":"moderator"}`))
	assignRR := httptest.NewRecorder()
	mux.ServeHTTP(assignRR, assignReq)
	if assignRR.Code != http.StatusOK {
		t.Fatalf("assign: expected 200, got %d: %s", assignRR.Code, assignRR.Body.String())
	}
	var assignBody map[string]any
	json.NewDecoder(assignRR.Body).Decode(&assignBody)
	if !strings.Contains(strings.Join(toStrings(assignBody["roles"]), ","), "moderator") {
		t.Fatalf("expected moderator to be granted, got %+v", assignBody)
	}

	revokeReq := httptest.NewRequest("POST", "/v1/authz/roles/revoke", strings.NewReader(`{"userId":"target","role":"moderator"}`))
	revokeRR := httptest.NewRecorder()
	mux.ServeHTTP(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", revokeRR.Code, revokeRR.Body.String())
	}
	var revokeBody map[string]any
	json.NewDecoder(revokeRR.Body).Decode(&revokeBody)
	if revokeBody["roles"] != nil && len(toStrings(revokeBody["roles"])) != 0 {
		t.Fatalf("expected no roles left after revoke, got %+v", revokeBody)
	}
}

func toStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
