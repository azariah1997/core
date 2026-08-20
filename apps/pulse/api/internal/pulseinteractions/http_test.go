package pulseinteractions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	pulseinteractionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

// newFakeCoreHTTPServer stands in for core-api's relationships list
// route - the one Core capability pulseinteractions' handler layer
// resolves per-caller. friends is served for every type query,
// matching whichever type the test wants connected.
func newFakeCoreHTTPServer(t *testing.T, relType, status string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != relType {
			json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
			{"requesterUserId": "caller-1", "targetUserId": "user-2", "status": status},
		}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func fixedCallerWithCore(id, serverURL string) func(http.Handler) http.Handler {
	client := coresdk.NewClient(serverURL, coresdk.WithTokenSource(coresdk.StaticTokenSource("fake-token")), coresdk.WithRetries(1, 0))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := pulseauth.WithCaller(r.Context(), id)
			ctx = pulseauth.WithClient(ctx, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newCoreFactory(pulseAppID string) pulseinteractions.CoreFactory {
	return func(client *coresdk.Client) pulseinteractions.CoreRelationships {
		return pulseinteractionscore.NewRelationshipsAdapter(client, pulseAppID)
	}
}

func TestCreateHandlerRejectsWithoutAConnection(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server := newFakeCoreHTTPServer(t, "pulse_friend", "")
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	req := httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFullPulseLifecycleThroughTheRealRouterAndAFakeCoreServer(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server := newFakeCoreHTTPServer(t, "pulse_friend", "active")
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	createReq := httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)

	startRR := httptest.NewRecorder()
	mux.ServeHTTP(startRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/start", nil))
	if startRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on start, got %d: %s", startRR.Code, startRR.Body.String())
	}
	if !strings.Contains(startRR.Body.String(), `"status":"started"`) {
		t.Fatalf("expected started status, got %s", startRR.Body.String())
	}

	time.Sleep(5 * time.Millisecond)
	stopRR := httptest.NewRecorder()
	mux.ServeHTTP(stopRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/stop", nil))
	if stopRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on stop, got %d: %s", stopRR.Code, stopRR.Body.String())
	}
	var stopped struct {
		Status     string `json:"status"`
		DurationMs int    `json:"durationMs"`
	}
	json.Unmarshal(stopRR.Body.Bytes(), &stopped)
	if stopped.Status != "completed" || stopped.DurationMs <= 0 {
		t.Fatalf("expected completed status with a real positive duration, got %+v", stopped)
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/pulse/interactions", nil))
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), created.ID) {
		t.Fatalf("expected the completed interaction in the list, got %d: %s", listRR.Code, listRR.Body.String())
	}
}

func TestStopBeforeStartReturnsConflict(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server := newFakeCoreHTTPServer(t, "pulse_friend", "active")
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`)))
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/stop", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}
