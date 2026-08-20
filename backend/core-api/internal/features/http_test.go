package features_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/features"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestCreateFeatureHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	features.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("POST", "/v1/features", strings.NewReader(`{"appId":"app-1","key":"x","name":"X"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateAndEvaluateFeatureRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	adminMux := http.NewServeMux()
	features.RegisterRoutes(adminMux, svc, fixedUser("admin"))

	createReq := httptest.NewRequest("POST", "/v1/features", strings.NewReader(`{"appId":"app-1","key":"x","name":"X","enabled":true}`))
	createRR := httptest.NewRecorder()
	adminMux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	ruleReq := httptest.NewRequest("POST", "/v1/features/app-1/x/rules", strings.NewReader(`{"priority":1,"enabled":true,"conditions":{}}`))
	ruleRR := httptest.NewRecorder()
	adminMux.ServeHTTP(ruleRR, ruleReq)
	if ruleRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", ruleRR.Code, ruleRR.Body.String())
	}

	readerMux := http.NewServeMux()
	features.RegisterRoutes(readerMux, svc, fixedUser("any-authenticated-user"))
	evalReq := httptest.NewRequest("POST", "/v1/features/app-1/x/evaluate", strings.NewReader(`{}`))
	evalRR := httptest.NewRecorder()
	readerMux.ServeHTTP(evalRR, evalReq)

	if evalRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", evalRR.Code, evalRR.Body.String())
	}
	if !strings.Contains(evalRR.Body.String(), `"enabled":true`) {
		t.Fatalf("expected the feature to evaluate enabled, got %s", evalRR.Body.String())
	}
}

func TestListFeaturesHandlerRequiresAppIDQueryParameter(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	features.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/features", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetFeatureHandlerReturns404ForUnknownFeature(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	features.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/features/app-1/does-not-exist", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
