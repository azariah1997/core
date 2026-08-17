package tenants_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/tenants"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: userID})))
		})
	}
}

func createTenant(t *testing.T, mux http.Handler, appID, slug, name string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/tenants",
		strings.NewReader(`{"appId":"`+appID+`","slug":"`+slug+`","name":"`+name+`"}`))
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

func TestCreateTenantEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	tenants.RegisterRoutes(mux, newService(), fixedUser("owner-1"))

	body := createTenant(t, mux, "app-1", "acme", "Acme")
	if body["slug"] != "acme" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestCreateTenantRejectsInvalidBody(t *testing.T) {
	mux := http.NewServeMux()
	tenants.RegisterRoutes(mux, newService(), fixedUser("owner-1"))

	req := httptest.NewRequest("POST", "/v1/tenants", strings.NewReader(`{"appId":"app-1","slug":"Not Valid!","name":"x"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListMineEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	svc := newService()
	tenants.RegisterRoutes(mux, svc, fixedUser("owner-1"))

	createTenant(t, mux, "app-1", "acme", "Acme")

	req := httptest.NewRequest("GET", "/v1/tenants", nil)
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
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(body.Items))
	}
}

func TestGetTenantDeniedForNonMember(t *testing.T) {
	svc := newService()
	ownerMux := http.NewServeMux()
	tenants.RegisterRoutes(ownerMux, svc, fixedUser("owner-1"))
	tenant := createTenant(t, ownerMux, "app-1", "acme", "Acme")

	strangerMux := http.NewServeMux()
	tenants.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))

	req := httptest.NewRequest("GET", "/v1/tenants/"+tenant["id"].(string), nil)
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAddAndListMembers(t *testing.T) {
	svc := newService()
	ownerMux := http.NewServeMux()
	tenants.RegisterRoutes(ownerMux, svc, fixedUser("owner-1"))
	tenant := createTenant(t, ownerMux, "app-1", "acme", "Acme")
	tenantID := tenant["id"].(string)

	addReq := httptest.NewRequest("POST", "/v1/tenants/"+tenantID+"/members",
		strings.NewReader(`{"userId":"member-1","role":"member"}`))
	addRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(addRR, addReq)
	if addRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", addRR.Code, addRR.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/v1/tenants/"+tenantID+"/members", nil)
	listRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(listRR, listReq)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 2 { // owner + new member
		t.Fatalf("expected 2 members, got %d: %+v", len(body.Items), body.Items)
	}
}

func TestRemoveMemberRequiresManagerUnlessSelf(t *testing.T) {
	svc := newService()
	ownerMux := http.NewServeMux()
	tenants.RegisterRoutes(ownerMux, svc, fixedUser("owner-1"))
	tenant := createTenant(t, ownerMux, "app-1", "acme", "Acme")
	tenantID := tenant["id"].(string)

	ownerMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/tenants/"+tenantID+"/members",
		strings.NewReader(`{"userId":"member-1","role":"member"}`)))

	memberMux := http.NewServeMux()
	tenants.RegisterRoutes(memberMux, svc, fixedUser("member-1"))

	removeReq := httptest.NewRequest("DELETE", "/v1/tenants/"+tenantID+"/members/member-1", nil)
	removeRR := httptest.NewRecorder()
	memberMux.ServeHTTP(removeRR, removeReq)
	if removeRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204 self-removal, got %d: %s", removeRR.Code, removeRR.Body.String())
	}
}
