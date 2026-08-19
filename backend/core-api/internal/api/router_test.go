package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/applications"
	applicationsmemory "github.com/example/core-platform/backend/core-api/internal/applications/memory"
	"github.com/example/core-platform/backend/core-api/internal/audit"
	auditmemory "github.com/example/core-platform/backend/core-api/internal/audit/memory"
	"github.com/example/core-platform/backend/core-api/internal/authz"
	authzmemory "github.com/example/core-platform/backend/core-api/internal/authz/memory"
	"github.com/example/core-platform/backend/core-api/internal/billing"
	billingmemory "github.com/example/core-platform/backend/core-api/internal/billing/memory"
	"github.com/example/core-platform/backend/core-api/internal/devices"
	devicesmemory "github.com/example/core-platform/backend/core-api/internal/devices/memory"
	"github.com/example/core-platform/backend/core-api/internal/features"
	featuresmemory "github.com/example/core-platform/backend/core-api/internal/features/memory"
	"github.com/example/core-platform/backend/core-api/internal/files"
	filesmemory "github.com/example/core-platform/backend/core-api/internal/files/memory"
	"github.com/example/core-platform/backend/core-api/internal/groups"
	groupsmemory "github.com/example/core-platform/backend/core-api/internal/groups/memory"
	"github.com/example/core-platform/backend/core-api/internal/identity"
	identitymemory "github.com/example/core-platform/backend/core-api/internal/identity/memory"
	"github.com/example/core-platform/backend/core-api/internal/jobs"
	jobsmemory "github.com/example/core-platform/backend/core-api/internal/jobs/memory"
	"github.com/example/core-platform/backend/core-api/internal/messaging"
	messagingmemory "github.com/example/core-platform/backend/core-api/internal/messaging/memory"
	"github.com/example/core-platform/backend/core-api/internal/notifications"
	notificationsmemory "github.com/example/core-platform/backend/core-api/internal/notifications/memory"
	"github.com/example/core-platform/backend/core-api/internal/privacy"
	privacymemory "github.com/example/core-platform/backend/core-api/internal/privacy/memory"
	"github.com/example/core-platform/backend/core-api/internal/relationships"
	relationshipsmemory "github.com/example/core-platform/backend/core-api/internal/relationships/memory"
	"github.com/example/core-platform/backend/core-api/internal/remoteconfig"
	remoteconfigmemory "github.com/example/core-platform/backend/core-api/internal/remoteconfig/memory"
	"github.com/example/core-platform/backend/core-api/internal/search"
	"github.com/example/core-platform/backend/core-api/internal/tenants"
	tenantsmemory "github.com/example/core-platform/backend/core-api/internal/tenants/memory"
	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
	trustsafetymemory "github.com/example/core-platform/backend/core-api/internal/trustsafety/memory"
	"github.com/example/core-platform/backend/core-api/internal/users"
	usersmemory "github.com/example/core-platform/backend/core-api/internal/users/memory"
	"github.com/example/core-platform/backend/core-api/internal/workflows"
	workflowsmemory "github.com/example/core-platform/backend/core-api/internal/workflows/memory"
	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

// noopRealtime satisfies messaging.Realtime without a real Redis - these
// router tests exercise HTTP wiring and cross-module auth, not realtime
// delivery (that's messaging's own package tests).
type noopRealtime struct{}

func (noopRealtime) ToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	return nil
}

// noopObjectStore satisfies files.ObjectStore without a real S3/MinIO -
// these router tests exercise HTTP wiring and cross-module auth, not
// object storage (that's files' own package tests, and this phase's live
// validation against real MinIO).
type noopObjectStore struct{}

func (noopObjectStore) PresignPut(ctx context.Context, objectKey, contentType string, expires time.Duration) (string, error) {
	return "https://noop.local/put/" + objectKey, nil
}
func (noopObjectStore) PresignGet(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	return "https://noop.local/get/" + objectKey, nil
}
func (noopObjectStore) HeadObject(ctx context.Context, objectKey string) (int64, string, error) {
	return 0, "", nil
}
func (noopObjectStore) DeleteObject(ctx context.Context, objectKey string) error { return nil }

// PutObject satisfies privacy.ExportStore alongside the PresignGet
// above - noopTemporalClient below already matches privacy.WorkflowStarter
// exactly, so neither test double needed a Phase-20-specific twin.
func (noopObjectStore) PutObject(ctx context.Context, objectKey string, body []byte, contentType string) error {
	return nil
}

// noopSearchProvider satisfies searchidx.Provider without a real
// OpenSearch - these router tests exercise HTTP wiring and cross-module
// auth, not indexing/search (that's search's own package tests, and this
// phase's live validation against real OpenSearch).
type noopSearchProvider struct{}

func (noopSearchProvider) Index(ctx context.Context, doc searchidx.Document) error { return nil }
func (noopSearchProvider) Delete(ctx context.Context, docType, appID, id string) error {
	return nil
}
func (noopSearchProvider) Query(ctx context.Context, params searchidx.QueryParams) (searchidx.QueryResult, error) {
	return searchidx.QueryResult{}, nil
}

// noopTemporalClient satisfies workflows.TemporalClient without a real
// Temporal server - these router tests exercise HTTP wiring and
// cross-module auth, not workflow execution (that's workflows' own
// package tests, and this phase's live validation against real Temporal).
type noopTemporalClient struct{}

func (noopTemporalClient) Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (string, error) {
	return "run-" + workflowID, nil
}
func (noopTemporalClient) Describe(ctx context.Context, workflowID, runID string) (workflows.Execution, error) {
	return workflows.Execution{Status: workflows.StatusRunning}, nil
}
func (noopTemporalClient) Signal(ctx context.Context, workflowID, runID, signalName string, payload map[string]any) error {
	return nil
}

// noopRateLimiter always allows - these router tests exercise HTTP
// wiring and cross-module auth, not rate-limiting behavior itself
// (that's trustsafety's own package tests, against a real miniredis-
// backed ratelimit.Limiter).
type noopRateLimiter struct{}

func (noopRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return true, nil
}

func newTestHandler() http.Handler {
	// Same two-phase wiring cmd/server/main.go uses to break the
	// authz<->audit construction cycle - see audit_adapter.go's doc
	// comment.
	roleChangeAuditRecorder := NewRoleChangeAuditRecorder()
	authzSvc := authz.NewService(authzmemory.NewRoleRepository(), authzmemory.NewProvider(), roleChangeAuditRecorder, slog.Default())
	auditSvc := audit.NewService(auditmemory.New(), authzSvc)
	roleChangeAuditRecorder.SetAuditService(auditSvc)

	usersSvc := users.NewService(usersmemory.New())
	devicesSvc := devices.NewService(devicesmemory.New())
	filesSvc := files.NewService(filesmemory.New(), noopObjectStore{}, authzSvc, files.Config{})

	// Same registration cmd/server/main.go performs, against the same
	// noop test doubles used elsewhere in this file - noopTemporalClient
	// satisfies privacy.WorkflowStarter and noopObjectStore satisfies
	// privacy.ExportStore with zero Phase-20-specific fakes needed.
	privacySvc := privacy.NewService(privacymemory.New(), authzSvc, noopTemporalClient{}, noopObjectStore{})
	usersPrivacyParticipant := NewUsersPrivacyParticipant(usersSvc)
	privacySvc.RegisterExporter("users", usersPrivacyParticipant)
	privacySvc.RegisterDeleter("users", usersPrivacyParticipant)
	devicesPrivacyParticipant := NewDevicesPrivacyParticipant(devicesSvc)
	privacySvc.RegisterExporter("devices", devicesPrivacyParticipant)
	privacySvc.RegisterDeleter("devices", devicesPrivacyParticipant)
	filesPrivacyParticipant := NewFilesPrivacyParticipant(filesSvc)
	privacySvc.RegisterExporter("files", filesPrivacyParticipant)
	privacySvc.RegisterDeleter("files", filesPrivacyParticipant)
	privacySvc.RegisterExporter("audit", NewAuditPrivacyParticipant(auditSvc))

	// authzSvc already satisfies trustsafety.ModeratorChecker directly
	// (IsPlatformAdmin plus IsModerator).
	trustSafetySvc := trustsafety.NewService(trustsafetymemory.New(), authzSvc, noopRateLimiter{})
	billingSvc := billing.NewService(billingmemory.New(), authzSvc)

	return New(
		config.Load(),
		applications.NewService(applicationsmemory.New()),
		identity.NewService("fake", identitymemory.Provider{}, identitymemory.New()),
		usersSvc,
		devicesSvc,
		authzSvc,
		tenants.NewService(tenantsmemory.New()),
		relationships.NewService(relationshipsmemory.New()),
		groups.NewService(groupsmemory.New()),
		messaging.NewService(messagingmemory.New(), noopRealtime{}, slog.Default()),
		notifications.NewService(notificationsmemory.New(), nil, authzSvc, slog.Default()),
		filesSvc,
		search.NewService(noopSearchProvider{}, authzSvc),
		jobs.NewService(jobsmemory.New(), authzSvc),
		workflows.NewService(workflowsmemory.New(), noopTemporalClient{}, authzSvc),
		features.NewService(featuresmemory.New(), authzSvc),
		remoteconfig.NewService(remoteconfigmemory.New(), authzSvc),
		auditSvc,
		privacySvc,
		trustSafetySvc,
		billingSvc,
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
	roleChangeAuditRecorder := NewRoleChangeAuditRecorder()
	authzSvc := authz.NewService(roles, authzmemory.NewProvider(), roleChangeAuditRecorder, slog.Default())
	auditSvc := audit.NewService(auditmemory.New(), authzSvc)
	roleChangeAuditRecorder.SetAuditService(auditSvc)
	privacySvc := privacy.NewService(privacymemory.New(), authzSvc, noopTemporalClient{}, noopObjectStore{})
	trustSafetySvc := trustsafety.NewService(trustsafetymemory.New(), authzSvc, noopRateLimiter{})
	billingSvc := billing.NewService(billingmemory.New(), authzSvc)
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
		notifications.NewService(notificationsmemory.New(), nil, authzSvc, slog.Default()),
		files.NewService(filesmemory.New(), noopObjectStore{}, authzSvc, files.Config{}),
		search.NewService(noopSearchProvider{}, authzSvc),
		jobs.NewService(jobsmemory.New(), authzSvc),
		workflows.NewService(workflowsmemory.New(), noopTemporalClient{}, authzSvc),
		features.NewService(featuresmemory.New(), authzSvc),
		remoteconfig.NewService(remoteconfigmemory.New(), authzSvc),
		auditSvc,
		privacySvc,
		trustSafetySvc,
		billingSvc,
	)

	otherUserID := provisionUser(t, handler, "someone-else")
	adminID := provisionUser(t, handler, "admin-user")

	if err := authzSvc.AssignRole(context.Background(), adminID, adminID, authz.RolePlatformAdmin); err != nil {
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
