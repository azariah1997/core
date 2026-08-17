package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/identity"
)

func newHandler() http.Handler {
	mux := http.NewServeMux()
	identity.RegisterRoutes(mux, newService())
	return mux
}

func TestMeWithoutTokenReturns401(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("GET", "/v1/identity/me", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMeWithMalformedHeaderReturns401(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("GET", "/v1/identity/me", nil)
	req.Header.Set("Authorization", "not-bearer-format")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMeWithValidTokenReturnsIdentity(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest("GET", "/v1/identity/me", nil)
	req.Header.Set("Authorization", "Bearer user-123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["providerSubject"] != "user-123" || body["provider"] != "fake" {
		t.Fatalf("unexpected body: %v", body)
	}
}
