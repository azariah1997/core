package privacy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/privacy"
	"github.com/example/core-platform/backend/core-api/internal/privacy/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeStarter struct {
	started []string
	fail    bool
}

func (s *fakeStarter) Start(ctx context.Context, workflowID, workflowType, taskQueue, cronSchedule string, payload map[string]any) (string, error) {
	if s.fail {
		return "", errors.New("temporal unreachable")
	}
	s.started = append(s.started, workflowType)
	return "run-" + workflowID, nil
}

type fakeStore struct {
	objects map[string][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (s *fakeStore) PutObject(ctx context.Context, objectKey string, body []byte, contentType string) error {
	s.objects[objectKey] = body
	return nil
}
func (s *fakeStore) PresignGet(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	return "https://fake.local/" + objectKey, nil
}

type fakeExporter struct {
	data map[string]any
	err  error
}

func (e fakeExporter) ExportUserData(ctx context.Context, userID string) (map[string]any, error) {
	return e.data, e.err
}

type fakeDeleter struct {
	called *bool
	err    error
}

func (d fakeDeleter) DeleteUserData(ctx context.Context, userID string) error {
	if d.called != nil {
		*d.called = true
	}
	return d.err
}

func newService(admins map[string]bool, starter privacy.WorkflowStarter, store privacy.ExportStore) *privacy.Service {
	return privacy.NewService(memory.New(), fakeAdmin{admins: admins}, starter, store)
}

func TestSetConsentRequiresPurpose(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	_, err := svc.SetConsent(context.Background(), "u1", privacy.ConsentInput{})
	var verr *privacy.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestConsentIsAppendOnlyHistory(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	ctx := context.Background()
	if _, err := svc.SetConsent(ctx, "u1", privacy.ConsentInput{Purpose: "marketing", Granted: true}); err != nil {
		t.Fatalf("set 1: %v", err)
	}
	if _, err := svc.SetConsent(ctx, "u1", privacy.ConsentInput{Purpose: "marketing", Granted: false}); err != nil {
		t.Fatalf("set 2: %v", err)
	}
	list, err := svc.ListConsent(ctx, "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected both consent rows to survive (append-only), got %d", len(list))
	}
	if list[0].Granted != false {
		t.Fatalf("expected the newest row (revoke) first, got %+v", list[0])
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	ctx := context.Background()
	prefs, err := svc.SetPreferences(ctx, "u1", map[string]bool{"data_sharing": false, "personalization": true})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if prefs["data_sharing"] != false || prefs["personalization"] != true {
		t.Fatalf("unexpected preferences: %+v", prefs)
	}
	got, err := svc.GetPreferences(ctx, "u1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got["data_sharing"] != false || got["personalization"] != true {
		t.Fatalf("unexpected preferences on re-read: %+v", got)
	}
}

func TestRetentionPolicyRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, &fakeStarter{}, newFakeStore())
	ctx := context.Background()
	if _, err := svc.CreateRetentionPolicy(ctx, "u1", privacy.RetentionPolicyInput{ResourceType: "message", RetentionDays: 30}); !errors.Is(err, privacy.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-admin, got %v", err)
	}
	p, err := svc.CreateRetentionPolicy(ctx, "admin", privacy.RetentionPolicyInput{ResourceType: "message", RetentionDays: 30})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.RetentionDays != 30 {
		t.Fatalf("unexpected policy: %+v", p)
	}
	if _, err := svc.ListRetentionPolicies(ctx, "u1"); !errors.Is(err, privacy.ErrForbidden) {
		t.Fatalf("expected ErrForbidden listing as a non-admin, got %v", err)
	}
	list, err := svc.ListRetentionPolicies(ctx, "admin")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one policy, got %d", len(list))
	}
}

func TestRequestExportStartsWorkflowAndTracksStatus(t *testing.T) {
	starter := &fakeStarter{}
	svc := newService(nil, starter, newFakeStore())
	ctx := context.Background()
	req, err := svc.RequestExport(ctx, "u1")
	if err != nil {
		t.Fatalf("request export: %v", err)
	}
	if req.WorkflowID == "" || req.RunID == "" {
		t.Fatalf("expected a populated workflow ref, got %+v", req)
	}
	if len(starter.started) != 1 || starter.started[0] != privacy.ExportWorkflowType {
		t.Fatalf("expected the export workflow type to be started, got %v", starter.started)
	}
	got, err := svc.GetExportRequest(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != privacy.StatusRunning {
		t.Fatalf("expected running status after a successful start, got %v", got.Status)
	}
}

func TestRequestExportFailsClosedWhenWorkflowStartFails(t *testing.T) {
	svc := newService(nil, &fakeStarter{fail: true}, newFakeStore())
	ctx := context.Background()
	_, err := svc.RequestExport(ctx, "u1")
	if err == nil {
		t.Fatal("expected an error when the workflow fails to start")
	}
}

func TestGetExportRequestRequiresOwnerOrAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true}, &fakeStarter{}, newFakeStore())
	ctx := context.Background()
	req, err := svc.RequestExport(ctx, "u1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.GetExportRequest(ctx, "u2", req.ID); !errors.Is(err, privacy.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger, got %v", err)
	}
	if _, err := svc.GetExportRequest(ctx, "admin", req.ID); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

func TestRunExportAssemblesBundleFromEveryExporterAndUploadsIt(t *testing.T) {
	store := newFakeStore()
	svc := newService(nil, &fakeStarter{}, store)
	svc.RegisterExporter("users", fakeExporter{data: map[string]any{"displayName": "Ada"}})
	svc.RegisterExporter("devices", fakeExporter{data: map[string]any{"devices": []string{"d1"}}})

	ctx := context.Background()
	req, err := svc.RequestExport(ctx, "u1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	result, err := svc.RunExport(ctx, req.ID, "u1")
	if err != nil {
		t.Fatalf("run export: %v", err)
	}
	if result["participants"] != 2 {
		t.Fatalf("expected 2 participants, got %+v", result)
	}
	if len(store.objects) != 1 {
		t.Fatalf("expected exactly one uploaded object, got %d", len(store.objects))
	}

	got, err := svc.GetExportRequest(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != privacy.StatusCompleted || got.ObjectKey == "" {
		t.Fatalf("expected a completed request with an object key, got %+v", got)
	}
	url, err := svc.GetExportDownloadURL(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("download url: %v", err)
	}
	if url == "" {
		t.Fatal("expected a non-empty download URL")
	}
}

func TestRunExportToleratesOneParticipantFailing(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	svc.RegisterExporter("users", fakeExporter{data: map[string]any{"displayName": "Ada"}})
	svc.RegisterExporter("broken", fakeExporter{err: errors.New("boom")})

	ctx := context.Background()
	req, err := svc.RequestExport(ctx, "u1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.RunExport(ctx, req.ID, "u1"); err != nil {
		t.Fatalf("expected RunExport to succeed overall despite one failing participant, got %v", err)
	}
	got, err := svc.GetExportRequest(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != privacy.StatusCompleted {
		t.Fatalf("expected the export to still complete, got %v", got.Status)
	}
}

func TestRunDeletionCallsEveryDeleterAndRecordsResults(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	var usersCalled, devicesCalled bool
	svc.RegisterDeleter("users", fakeDeleter{called: &usersCalled})
	svc.RegisterDeleter("devices", fakeDeleter{called: &devicesCalled})

	ctx := context.Background()
	req, err := svc.RequestDeletion(ctx, "u1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.RunDeletion(ctx, req.ID, "u1"); err != nil {
		t.Fatalf("run deletion: %v", err)
	}
	if !usersCalled || !devicesCalled {
		t.Fatalf("expected every registered deleter to be called, users=%v devices=%v", usersCalled, devicesCalled)
	}
	got, err := svc.GetDeletionRequest(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != privacy.StatusCompleted {
		t.Fatalf("expected completed status, got %v", got.Status)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected a result entry per deleter, got %+v", got.Results)
	}
}

func TestRunDeletionRecordsPartialFailureButStillRunsEveryDeleter(t *testing.T) {
	svc := newService(nil, &fakeStarter{}, newFakeStore())
	var devicesCalled bool
	svc.RegisterDeleter("files", fakeDeleter{err: errors.New("storage unavailable")})
	svc.RegisterDeleter("devices", fakeDeleter{called: &devicesCalled})

	ctx := context.Background()
	req, err := svc.RequestDeletion(ctx, "u1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.RunDeletion(ctx, req.ID, "u1"); err != nil {
		t.Fatalf("run deletion: %v", err)
	}
	if !devicesCalled {
		t.Fatal("expected the devices deleter to still run despite the files deleter failing")
	}
	got, err := svc.GetDeletionRequest(ctx, "u1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != privacy.StatusFailed {
		t.Fatalf("expected the overall request to be marked failed, got %v", got.Status)
	}
}

// There is deliberately no way to register audit (or anything) as a
// Deleter and have RunDeletion skip it silently - Exporter and Deleter
// are separate interfaces precisely so a participant that should never
// be erased simply never implements Deleter at all. See
// internal/api/privacy_adapters.go's auditPrivacyParticipant.
