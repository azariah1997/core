package signals_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/signals"
	signalscore "github.com/example/core-platform/apps/pulse/api/internal/signals/core"
	"github.com/example/core-platform/apps/pulse/api/internal/signals/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

func fakeRelationshipsServer(t *testing.T, items []map[string]any) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func relItem(id, requesterID, targetID, status string) map[string]any {
	now := "2026-01-01T00:00:00.000Z"
	return map[string]any{"id": id, "requesterUserId": requesterID, "targetUserId": targetID, "status": status, "createdAt": now, "updatedAt": now}
}

func fixedCaller(id, serverURL string) func(http.Handler) http.Handler {
	client := coresdk.NewClient(serverURL, coresdk.WithTokenSource(coresdk.StaticTokenSource("fake-token")), coresdk.WithRetries(1, 0))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := pulseauth.WithCaller(r.Context(), id)
			ctx = pulseauth.WithClient(ctx, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newRouterFor(callerID, serverURL string, svc *signals.Service) *http.ServeMux {
	mux := http.NewServeMux()
	newCore := func(client *coresdk.Client) signals.CoreRelationships {
		return signalscore.NewRelationshipsAdapter(client, "app-1")
	}
	signals.RegisterRoutes(mux, svc, newCore, fixedCaller(callerID, serverURL))
	return mux
}

func TestCreateSignalHandlerRejectsWithoutAConnection(t *testing.T) {
	server := fakeRelationshipsServer(t, nil)
	mux := newRouterFor("caller-1", server.URL, signals.NewService(memory.New()))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/signals", strings.NewReader(`{"targetUserId":"user-2","segments":[{"type":"tap","durationMs":150}]}`)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSignalCRUDRoundTripsThroughTheRealRouter(t *testing.T) {
	server := fakeRelationshipsServer(t, []map[string]any{relItem("rel-1", "caller-1", "user-2", "active")})
	mux := newRouterFor("caller-1", server.URL, signals.NewService(memory.New()))

	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/signals", strings.NewReader(
		`{"targetUserId":"user-2","label":"I love you","segments":[{"type":"tap","durationMs":150},{"type":"pause","durationMs":300},{"type":"hold","durationMs":900}]}`,
	)))
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Label    string `json:"label"`
		Segments []struct {
			Type       string `json:"type"`
			DurationMs int    `json:"durationMs"`
		} `json:"segments"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)
	if created.Label != "I love you" || len(created.Segments) != 3 {
		t.Fatalf("expected the real label/segments echoed back, got %+v", created)
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/pulse/signals", nil))
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), created.ID) {
		t.Fatalf("expected the created signal in the list, got %d: %s", listRR.Code, listRR.Body.String())
	}

	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest("GET", "/v1/pulse/signals/"+created.ID, nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d: %s", getRR.Code, getRR.Body.String())
	}

	deleteRR := httptest.NewRecorder()
	mux.ServeHTTP(deleteRR, httptest.NewRequest("DELETE", "/v1/pulse/signals/"+created.ID, nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}

	getAfterDeleteRR := httptest.NewRecorder()
	mux.ServeHTTP(getAfterDeleteRR, httptest.NewRequest("GET", "/v1/pulse/signals/"+created.ID, nil))
	if getAfterDeleteRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d: %s", getAfterDeleteRR.Code, getAfterDeleteRR.Body.String())
	}
}

func TestGetSignalHandlerForbidsANonOwner(t *testing.T) {
	server := fakeRelationshipsServer(t, []map[string]any{relItem("rel-1", "caller-1", "user-2", "active")})
	svc := signals.NewService(memory.New())
	ownerMux := newRouterFor("caller-1", server.URL, svc)

	createRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/signals", strings.NewReader(`{"targetUserId":"user-2","segments":[{"type":"tap","durationMs":150}]}`)))
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// The signal's own bound target is not its owner, and may not read it.
	targetMux := newRouterFor("user-2", server.URL, svc)
	rr := httptest.NewRecorder()
	targetMux.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pulse/signals/"+created.ID, nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (the signal's target is not its owner), got %d: %s", rr.Code, rr.Body.String())
	}
}
