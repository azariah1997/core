package relationships_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/relationships"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: userID})))
		})
	}
}

func TestRequestAcceptEndToEnd(t *testing.T) {
	svc := newService()
	u1Mux := http.NewServeMux()
	relationships.RegisterRoutes(u1Mux, svc, fixedUser("u1"))
	u2Mux := http.NewServeMux()
	relationships.RegisterRoutes(u2Mux, svc, fixedUser("u2"))

	createReq := httptest.NewRequest("POST", "/v1/relationships",
		strings.NewReader(`{"appId":"app-1","targetUserId":"u2","type":"friend"}`))
	createRR := httptest.NewRecorder()
	u1Mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id := created["id"].(string)

	acceptReq := httptest.NewRequest("POST", "/v1/relationships/"+id+"/accept", nil)
	acceptRR := httptest.NewRecorder()
	u2Mux.ServeHTTP(acceptRR, acceptReq)
	if acceptRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}
	var accepted map[string]any
	if err := json.NewDecoder(acceptRR.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if accepted["status"] != "active" {
		t.Fatalf("expected active, got %v", accepted["status"])
	}
}

func TestRequesterCannotAcceptOwnRequest(t *testing.T) {
	svc := newService()
	u1Mux := http.NewServeMux()
	relationships.RegisterRoutes(u1Mux, svc, fixedUser("u1"))

	createRR := httptest.NewRecorder()
	u1Mux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/relationships",
		strings.NewReader(`{"appId":"app-1","targetUserId":"u2","type":"friend"}`)))
	var created map[string]any
	_ = json.NewDecoder(createRR.Body).Decode(&created)
	id := created["id"].(string)

	acceptRR := httptest.NewRecorder()
	u1Mux.ServeHTTP(acceptRR, httptest.NewRequest("POST", "/v1/relationships/"+id+"/accept", nil))
	if acceptRR.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}
}

func TestBlockEndpointEndToEnd(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	relationships.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/relationships/block",
		strings.NewReader(`{"appId":"app-1","targetUserId":"u2","type":"friend"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "blocked" {
		t.Fatalf("expected blocked, got %v", body["status"])
	}
}

func TestSecondRequestReturns409(t *testing.T) {
	svc := newService()
	mux := http.NewServeMux()
	relationships.RegisterRoutes(mux, svc, fixedUser("u1"))

	body := `{"appId":"app-1","targetUserId":"u2","type":"friend"}`
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/relationships", strings.NewReader(body)))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/relationships", strings.NewReader(body)))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
