package groups_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/groups"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: userID})))
		})
	}
}

func createGroup(t *testing.T, mux http.Handler, appID, name string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/groups", strings.NewReader(`{"appId":"`+appID+`","name":"`+name+`"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestCreateGroupEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	groups.RegisterRoutes(mux, newService(), fixedUser("creator"))

	body := createGroup(t, mux, "app-1", "Guild")
	if body["name"] != "Guild" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestAddMemberWithFreeFormRoleEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	groups.RegisterRoutes(mux, newService(), fixedUser("creator"))
	g := createGroup(t, mux, "app-1", "Family")

	req := httptest.NewRequest("POST", "/v1/groups/"+g["id"].(string)+"/members",
		strings.NewReader(`{"userId":"kid","role":"child"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["role"] != "child" || body["isManager"] != false {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestNonManagerCannotUpdateGroup(t *testing.T) {
	svc := newService()
	ownerMux := http.NewServeMux()
	groups.RegisterRoutes(ownerMux, svc, fixedUser("creator"))
	g := createGroup(t, ownerMux, "app-1", "Team")
	groupID := g["id"].(string)

	ownerMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/groups/"+groupID+"/members",
		strings.NewReader(`{"userId":"member-1"}`)))

	memberMux := http.NewServeMux()
	groups.RegisterRoutes(memberMux, svc, fixedUser("member-1"))

	req := httptest.NewRequest("PATCH", "/v1/groups/"+groupID, strings.NewReader(`{"name":"Hacked"}`))
	rr := httptest.NewRecorder()
	memberMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestLeaveGroupEndToEnd(t *testing.T) {
	svc := newService()
	ownerMux := http.NewServeMux()
	groups.RegisterRoutes(ownerMux, svc, fixedUser("creator"))
	g := createGroup(t, ownerMux, "app-1", "Team")
	groupID := g["id"].(string)

	ownerMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/groups/"+groupID+"/members",
		strings.NewReader(`{"userId":"member-1"}`)))

	memberMux := http.NewServeMux()
	groups.RegisterRoutes(memberMux, svc, fixedUser("member-1"))

	req := httptest.NewRequest("DELETE", "/v1/groups/"+groupID+"/members/member-1", nil)
	rr := httptest.NewRecorder()
	memberMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
