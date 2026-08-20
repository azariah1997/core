package search_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestIndexHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := search.NewService(newFakeProvider(), fakeAdmin{})
	mux := http.NewServeMux()
	search.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("POST", "/v1/search/index", strings.NewReader(`{"type":"user","id":"u1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestIndexAndQueryRoundTripThroughTheRealRouter(t *testing.T) {
	svc := search.NewService(newFakeProvider(), fakeAdmin{admins: map[string]bool{"admin": true}})
	adminMux := http.NewServeMux()
	search.RegisterRoutes(adminMux, svc, fixedUser("admin"))

	indexReq := httptest.NewRequest("POST", "/v1/search/index", strings.NewReader(`{"type":"user","id":"u1","fields":{"displayName":"Alice"}}`))
	indexRR := httptest.NewRecorder()
	adminMux.ServeHTTP(indexRR, indexReq)
	if indexRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", indexRR.Code, indexRR.Body.String())
	}

	readerMux := http.NewServeMux()
	search.RegisterRoutes(readerMux, svc, fixedUser("any-authenticated-user"))
	queryReq := httptest.NewRequest("GET", "/v1/search?type=user", nil)
	queryRR := httptest.NewRecorder()
	readerMux.ServeHTTP(queryRR, queryReq)

	if queryRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", queryRR.Code, queryRR.Body.String())
	}
	if !strings.Contains(queryRR.Body.String(), "Alice") {
		t.Fatalf("expected the indexed document in the response, got %s", queryRR.Body.String())
	}
}

func TestDeleteHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := search.NewService(newFakeProvider(), fakeAdmin{admins: map[string]bool{"admin": true}})
	mux := http.NewServeMux()
	search.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("DELETE", "/v1/search/index/user/u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}
