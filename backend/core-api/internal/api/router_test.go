package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationsmemory "github.com/example/core-platform/backend/core-api/internal/applications/memory"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	authzmemory "github.com/example/core-platform/backend/core-api/internal/authz/memory"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	devicesmemory "github.com/example/core-platform/backend/core-api/internal/devices/memory"
	"github.com/example/core-platform/backend/core-api/internal/groups"
	groupsmemory "github.com/example/core-platform/backend/core-api/internal/groups/memory"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	identitymemory "github.com/example/core-platform/backend/core-api/internal/identity/memory"
	"github.com/example/core-platform/backend/core-api/internal/messaging"
	messagingmemory "github.com/example/core-platform/backend/core-api/internal/messaging/memory"
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	relationshipsmemory "github.com/example/core-platform/backend/core-api/internal/relationships/memory"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	tenantsmemory "github.com/example/core-platform/backend/core-api/internal/tenants/memory"
	"github.com/example/core-platform/backend/core-api/internal/users"
	usersmemory "github.com/example/core-platform/backend/core-api/internal/users/memory"
	"github.com/example/core-platform/packages/go/platformkit/config"
)

// noopRealtime satisfies messaging.Realtime without a real Redis - these
// router tests exercise HTTP wiring and cross-module auth, not realtime
// delivery (that's messaging's own package tests).
type noopRealtime struct{}

func (noopRealtime) ToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	return nil
}

func newTestHandler() http.Handler {
	return New(
		config.Load(),
		applications.NewService(applicationsmemory.New()),
		identity.NewService("fake", identitymemory.Provider{}, identitymemory.New()),
		users.NewService(usersmemory.New()),
		devices.NewService(devicesmemory.New()),
		authz.NewService(authzmemory.NewRoleRepository(), authzmemory.NewProvider()),
		tenants.NewService(tenantsmemory.New()),
		relationships.NewService(relationshipsmemory.New()),
		groups.NewService(groupsmemory.New()),
		messaging.NewService(messagingmemory.New(), noopRealtime{}, slog.Default()),
	)
}

func TestLiveness(t *testing.T) {
	req := httptest.NewRequest("GET", "/livez", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGenericDataAPIIsBlocked(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/data/query", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 501 {
		t.Fatalf("expected 501, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "NOT_IMPLEMENTED" {
		t.Fatalf("expected standard error envelope with code NOT_IMPLEMENTED, got %v", body)
	}
}

func TestUnmatchedRouteReturnsStandardNotFoundEnvelope(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/does-not-exist", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "RESOURCE_NOT_FOUND" {
		t.Fatalf("expected standard error envelope with code RESOURCE_NOT_FOUND, got %v", body)
	}
}

func TestCorrelationIDIsReflectedOnResponse(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/platform", nil)
	req.Header.Set("X-Correlation-ID", "test-correlation-id")
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Correlation-ID"); got != "test-correlation-id" {
		t.Fatalf("expected correlation ID to be echoed back, got %q", got)
	}
}

func TestCreateAndGetApplicationEndToEnd(t *testing.T) {
	handler := newTestHandler()

	createReq := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"demo-app","name":"Demo App"}`))
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)
	if createRR.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("expected created application to have an id")
	}

	getReq := httptest.NewRequest("GET", "/v1/apps/"+id, nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
}

func TestCreateApplicationRejectsInvalidSlug(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/apps", strings.NewReader(`{"slug":"Not Valid!","name":"x"}`))
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMeWithoutTokenReturns401(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/users/me", nil)
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// TestFirstLoginProvisionsAndLinksAUser is the Phase 3 + Phase 4 seam:
// an authenticated identity with no linked user yet should get one
// created and linked on its first authenticated request, and every
// subsequent request should resolve to that same user.
func TestFirstLoginProvisionsAndLinksAUser(t *testing.T) {
	handler := newTestHandler()

	first := httptest.NewRequest("GET", "/v1/users/me", nil)
	first.Header.Set("Authorization", "Bearer newcomer")
	firstRR := httptest.NewRecorder()
	handler.ServeHTTP(firstRR, first)
	if firstRR.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", firstRR.Code, firstRR.Body.String())
	}
	var firstBody map[string]any
	if err := json.NewDecoder(firstRR.Body).Decode(&firstBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	userID, _ := firstBody["id"].(string)
	if userID == "" {
		t.Fatal("expected a provisioned user id")
	}
	if firstBody["displayName"] != "newcomer" {
		t.Fatalf("expected displayName derived from the fake provider's username, got %v", firstBody["displayName"])
	}

	second := httptest.NewRequest("GET", "/v1/users/me", nil)
	second.Header.Set("Authorization", "Bearer newcomer")
	secondRR := httptest.NewRecorder()
	handler.ServeHTTP(secondRR, second)
	var secondBody map[string]any
	if err := json.NewDecoder(secondRR.Body).Decode(&secondBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if secondBody["id"] != userID {
		t.Fatalf("expected the same user to be resolved on a second login, got %v vs %v", secondBody["id"], userID)
	}
}

func TestPatchMeRequiresAuth(t *testing.T) {
	req := httptest.NewRequest("PATCH", "/v1/users/me", strings.NewReader(`{"displayName":"x"}`))
	rr := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func provisionUser(t *testing.T, handler http.Handler, token string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode provisioned user: %v", err)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("expected a provisioned user id, got body %v", body)
	}
	return id
}

func TestGetUserByIDRequiresAuth(t *testing.T) {
	handler := newTestHandler()
	otherUserID := provisionUser(t, handler, "someone-else")

	req := httptest.NewRequest("GET", "/v1/users/"+otherUserID, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestGetUserByIDAllowsSelfAccess(t *testing.T) {
	handler := newTestHandler()
	ownID := provisionUser(t, handler, "newcomer")

	req := httptest.NewRequest("GET", "/v1/users/"+ownID, nil)
	req.Header.Set("Authorization", "Bearer newcomer")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200 viewing own profile via /v1/users/{id}, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetUserByIDDeniesCrossUserAccessWithoutRole(t *testing.T) {
	handler := newTestHandler()
	otherUserID := provisionUser(t, handler, "someone-else")
	provisionUser(t, handler, "newcomer")

	req := httptest.NewRequest("GET", "/v1/users/"+otherUserID, nil)
	req.Header.Set("Authorization", "Bearer newcomer")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("expected 403 viewing another user's profile without a role, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["code"] != "ACCESS_DENIED" {
		t.Fatalf("expected ACCESS_DENIED envelope, got %v", body)
	}
}

// TestGetUserByIDAllowsPlatformAdminCrossUserAccess is the Phase 6 seam:
// RBAC (platform.admin) and the fine-grained provider (OpenFGA in
// production, the in-memory fake here) combine through authz.Service to
// grant access no plain authenticated caller has.
func TestGetUserByIDAllowsPlatformAdminCrossUserAccess(t *testing.T) {
	roles := authzmemory.NewRoleRepository()
	authzSvc := authz.NewService(roles, authzmemory.NewProvider())
	handler := New(
		config.Load(),
		applications.NewService(applicationsmemory.New()),
		identity.NewService("fake", identitymemory.Provider{}, identitymemory.New()),
		users.NewService(usersmemory.New()),
		devices.NewService(devicesmemory.New()),
		authzSvc,
		tenants.NewService(tenantsmemory.New()),
		relationships.NewService(relationshipsmemory.New()),
		groups.NewService(groupsmemory.New()),
		messaging.NewService(messagingmemory.New(), noopRealtime{}, slog.Default()),
	)

	otherUserID := provisionUser(t, handler, "someone-else")
	adminID := provisionUser(t, handler, "admin-user")

	if err := authzSvc.AssignRole(context.Background(), adminID, authz.RolePlatformAdmin); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	req := httptest.NewRequest("GET", "/v1/users/"+otherUserID, nil)
	req.Header.Set("Authorization", "Bearer admin-user")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200 for a platform.admin viewing another user's profile, got %d: %s", rr.Code, rr.Body.String())
	}
}
