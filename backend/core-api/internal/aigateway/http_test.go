package aigateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

// fixedUser is the same fixture pattern users/http_test.go establishes -
// every module in this package attaches a caller via users.WithUser and
// reads it back via users.FromContext (see each module's own callerID
// helper), so this same middleware shape works unmodified everywhere.
func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestCompleteHandlerReturns400ForMissingModelAlias(t *testing.T) {
	svc, _ := newService(nil, true)
	mux := http.NewServeMux()
	aigateway.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/ai/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCompleteHandlerSucceedsThroughTheRealRouter(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "ollama", result: aigateway.ProviderResult{Text: "hello", PromptTokens: 3, CompletionTokens: 2}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})

	mux := http.NewServeMux()
	aigateway.RegisterRoutes(mux, svc, fixedUser("admin"))

	req := httptest.NewRequest("POST", "/v1/ai/completions", strings.NewReader(`{"modelAlias":"default","messages":[{"role":"user","content":"hi"}]}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["text"] != "hello" {
		t.Fatalf("expected the completion text in the response, got %+v", body)
	}
}

func TestListUsageHandlerReturns403ForAStranger(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "ollama", result: aigateway.ProviderResult{Text: "hi"}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "m1"}}})

	mux := http.NewServeMux()
	aigateway.RegisterRoutes(mux, svc, fixedUser("owner"))
	req := httptest.NewRequest("POST", "/v1/ai/completions", strings.NewReader(`{"modelAlias":"default","messages":[{"role":"user","content":"hi"}]}`))
	mux.ServeHTTP(httptest.NewRecorder(), req)

	stranger := http.NewServeMux()
	aigateway.RegisterRoutes(stranger, svc, fixedUser("stranger"))
	getReq := httptest.NewRequest("GET", "/v1/ai/usage?userId=owner", nil)
	rr := httptest.NewRecorder()
	stranger.ServeHTTP(rr, getReq)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListModelsHandlerReturnsRegisteredAliases(t *testing.T) {
	svc, _ := newService(nil, true)
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})

	mux := http.NewServeMux()
	aigateway.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/ai/models", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "default") {
		t.Fatalf("expected the registered alias in the response, got %s", rr.Body.String())
	}
}
