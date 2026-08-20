package billing_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/billing"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestGrantEntitlementHandlerRequiresPlatformAdmin(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	billing.RegisterRoutes(mux, svc, fixedUser("not-admin"))

	req := httptest.NewRequest("POST", "/v1/billing/entitlements", strings.NewReader(`{"userId":"target","key":"premium_tier"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGrantAndCheckEntitlementRoundTripThroughTheRealRouter(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	mux := http.NewServeMux()
	billing.RegisterRoutes(mux, svc, fixedUser("admin"))

	grantReq := httptest.NewRequest("POST", "/v1/billing/entitlements", strings.NewReader(`{"userId":"target","key":"premium_tier"}`))
	grantRR := httptest.NewRecorder()
	mux.ServeHTTP(grantRR, grantReq)
	if grantRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", grantRR.Code, grantRR.Body.String())
	}

	targetMux := http.NewServeMux()
	billing.RegisterRoutes(targetMux, svc, fixedUser("target"))
	checkReq := httptest.NewRequest("GET", "/v1/billing/entitlements/check/premium_tier", nil)
	checkRR := httptest.NewRecorder()
	targetMux.ServeHTTP(checkRR, checkReq)

	if checkRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", checkRR.Code, checkRR.Body.String())
	}
	if !strings.Contains(checkRR.Body.String(), `"active":true`) {
		t.Fatalf("expected the granted entitlement to be active, got %s", checkRR.Body.String())
	}
}

func TestWebhookHandlerRejectsAnInvalidSignatureWithNoAuthMiddlewareAtAll(t *testing.T) {
	// RegisterWebhookRoute deliberately wires no requireUser at all -
	// Stripe's own signature is the authentication, not a Bearer token.
	svc := newService(nil)
	svc.RegisterProvider(fakeProvider{name: "stripe", events: map[string]billing.WebhookEvent{}})

	mux := http.NewServeMux()
	billing.RegisterWebhookRoute(mux, svc)

	req := httptest.NewRequest("POST", "/v1/billing/webhooks/stripe", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "not-a-real-signature")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("expected an invalid signature to be rejected, got 200: %s", rr.Body.String())
	}
}

func TestListPaymentsHandlerReturnsEmptyForANewUser(t *testing.T) {
	svc := newService(nil)
	mux := http.NewServeMux()
	billing.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("GET", "/v1/billing/payments", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Fatalf("expected an empty items list, got %s", rr.Body.String())
	}
}
