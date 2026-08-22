package pulseinteractions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions"
	pulseinteractionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseinteractions/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeCoreServer stands in for both core-api (relationships list,
// notifications send) and realtime-gateway (presence) - one fake
// server plays both roles in tests, since only the routes matter, not
// which real service would host them.
type fakeCoreServer struct {
	mu                sync.Mutex
	notificationsSent []string // receiverUserId
}

func newFakeCoreServer(t *testing.T, relType, relStatus string, online bool) (*httptest.Server, *fakeCoreServer) {
	f := &fakeCoreServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != relType {
			json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
			{"requesterUserId": "caller-1", "targetUserId": "user-2", "status": relStatus},
		}})
	})
	mux.HandleFunc("GET /v1/presence/{userId}", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"userId": r.PathValue("userId"), "online": online})
	})
	mux.HandleFunc("POST /v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserID string `json:"userId"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.notificationsSent = append(f.notificationsSent, body.UserID)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"notification": map[string]any{"id": "notif-1"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, f
}

func fixedCallerWithCore(id, serverURL string) func(http.Handler) http.Handler {
	client := coresdk.NewClient(serverURL, coresdk.WithTokenSource(coresdk.StaticTokenSource("fake-token")), coresdk.WithRetries(1, 0))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := pulseauth.WithCaller(r.Context(), id)
			ctx = pulseauth.WithClient(ctx, client)
			ctx = pulseauth.WithToken(ctx, "fake-token")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newCoreFactory(pulseAppID string) pulseinteractions.CoreFactory {
	return func(client *coresdk.Client) pulseinteractions.CoreRelationships {
		return pulseinteractionscore.NewRelationshipsAdapter(client, pulseAppID)
	}
}

func newPresenceFactory(realtimeAPIURL string) pulseinteractions.PresenceFactory {
	return func(token string) pulseinteractions.Presence {
		return pulseinteractionscore.NewPresenceAdapter(realtimeAPIURL, token)
	}
}

func newNotifierFactory(pulseAppID string) pulseinteractions.NotifierFactory {
	return func(client *coresdk.Client) pulseinteractions.Notifier {
		return pulseinteractionscore.NewNotifierAdapter(client, pulseAppID)
	}
}

// fakeSignals is unused by every test in this file except SendSignal's
// own (see http_test.go's dedicated signal tests below) - a fixed,
// service-level dependency, same as the real signals.Service would be.
type fakeSignals struct {
	ref pulseinteractions.SignalRef
	err error
}

func (f fakeSignals) Get(ctx context.Context, callerID, signalID string) (pulseinteractions.SignalRef, error) {
	return f.ref, f.err
}

func TestCreateHandlerRejectsWithoutAConnection(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, _ := newFakeCoreServer(t, "pulse_friend", "", true)
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

	req := httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFullLivePulseLifecycleThroughTheRealRouterAndAFakeCoreServer(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, fakeCore := newFakeCoreServer(t, "pulse_friend", "active", true) // receiver online
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

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
	if !strings.Contains(startRR.Body.String(), `"status":"started"`) || !strings.Contains(startRR.Body.String(), `"deliveryMode":"live"`) {
		t.Fatalf("expected started status with live delivery mode, got %s", startRR.Body.String())
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

	fakeCore.mu.Lock()
	notified := len(fakeCore.notificationsSent)
	fakeCore.mu.Unlock()
	if notified != 0 {
		t.Fatalf("expected no push notification for a live-delivered Pulse, got %d", notified)
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/pulse/interactions", nil))
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), created.ID) {
		t.Fatalf("expected the completed interaction in the list, got %d: %s", listRR.Code, listRR.Body.String())
	}
}

func TestOfflineReceiverGetsAPushNotificationThroughTheRealRouter(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, fakeCore := newFakeCoreServer(t, "pulse_friend", "active", false) // receiver offline
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`)))
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)

	startRR := httptest.NewRecorder()
	mux.ServeHTTP(startRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/start", nil))
	if !strings.Contains(startRR.Body.String(), `"deliveryMode":"push"`) {
		t.Fatalf("expected push delivery mode for an offline receiver, got %s", startRR.Body.String())
	}

	stopRR := httptest.NewRecorder()
	mux.ServeHTTP(stopRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/stop", nil))
	if stopRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on stop, got %d: %s", stopRR.Code, stopRR.Body.String())
	}

	fakeCore.mu.Lock()
	defer fakeCore.mu.Unlock()
	if len(fakeCore.notificationsSent) != 1 || fakeCore.notificationsSent[0] != "user-2" {
		t.Fatalf("expected exactly one push notification to user-2, got %v", fakeCore.notificationsSent)
	}
}

func TestPulseBackHandlerRoundTripsThroughTheRealRouter(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, _ := newFakeCoreServer(t, "pulse_friend", "active", true)

	senderMux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(senderMux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

	createRR := httptest.NewRecorder()
	senderMux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`)))
	var original struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &original)
	senderMux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/pulse/interactions/"+original.ID+"/start", nil))
	stopRR := httptest.NewRecorder()
	senderMux.ServeHTTP(stopRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+original.ID+"/stop", nil))
	if stopRR.Code != http.StatusOK {
		t.Fatalf("expected the original Pulse to complete, got %d: %s", stopRR.Code, stopRR.Body.String())
	}

	// user-2 (the real receiver) Pulses Back - one API call.
	receiverMux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(receiverMux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("user-2", server.URL))

	backRR := httptest.NewRecorder()
	receiverMux.ServeHTTP(backRR, httptest.NewRequest("POST", "/v1/pulse/interactions/"+original.ID+"/pulse-back", nil))
	if backRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", backRR.Code, backRR.Body.String())
	}
	var back struct {
		Status         string `json:"status"`
		Role           string `json:"role"`
		OtherUserID    string `json:"otherUserId"`
		InResponseToID string `json:"inResponseToId"`
	}
	json.Unmarshal(backRR.Body.Bytes(), &back)
	if back.Status != "completed" {
		t.Fatalf("expected PulseBack to complete in one call, got %+v", back)
	}
	if back.Role != "sender" || back.OtherUserID != "caller-1" {
		t.Fatalf("expected user-2 to be the sender of the reciprocal Pulse back to caller-1, got %+v", back)
	}
	if back.InResponseToID != original.ID {
		t.Fatalf("expected inResponseToId to link back to the original, got %+v", back)
	}
}

func TestPulseBackRejectsAnIncompleteOriginalThroughTheRealRouter(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, _ := newFakeCoreServer(t, "pulse_friend", "active", true)

	senderMux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(senderMux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))
	createRR := httptest.NewRecorder()
	senderMux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/interactions", strings.NewReader(`{"type":"pulse","receiverId":"user-2"}`)))
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)
	// Deliberately never started/stopped - still just CREATED.

	// The real receiver (user-2) tries to Pulse Back on an interaction
	// that never actually completed.
	receiverMux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(receiverMux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("user-2", server.URL))
	rr := httptest.NewRecorder()
	receiverMux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/interactions/"+created.ID+"/pulse-back", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for an incomplete original, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestKnockHandlerRoundTripsThroughTheRealRouter(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, fakeCore := newFakeCoreServer(t, "pulse_friend", "active", false) // receiver offline
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/knocks", strings.NewReader(`{"receiverId":"user-2","pattern":"triple_tap"}`)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var knock struct {
		Type       string `json:"type"`
		Status     string `json:"status"`
		Pattern    string `json:"pattern"`
		DurationMs int    `json:"durationMs"`
	}
	json.Unmarshal(rr.Body.Bytes(), &knock)
	if knock.Type != "knock" || knock.Status != "completed" || knock.Pattern != "triple_tap" {
		t.Fatalf("expected a completed knock with the requested pattern, got %+v", knock)
	}

	fakeCore.mu.Lock()
	defer fakeCore.mu.Unlock()
	if len(fakeCore.notificationsSent) != 1 || fakeCore.notificationsSent[0] != "user-2" {
		t.Fatalf("expected exactly one push notification to user-2, got %v", fakeCore.notificationsSent)
	}
}

func TestSendSignalHandlerRoundTripsThroughTheRealRouter(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, fakeCore := newFakeCoreServer(t, "pulse_friend", "active", false) // receiver offline
	sig := fakeSignals{ref: pulseinteractions.SignalRef{
		ID: "signal-1", TargetUserID: "user-2",
		Segments: []pulseinteractions.Segment{{Type: "tap", DurationMs: 150}, {Type: "hold", DurationMs: 900}},
	}}
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), sig, fixedCallerWithCore("caller-1", server.URL))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/signals/signal-1/send", nil))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var sent struct {
		Type        string `json:"type"`
		Status      string `json:"status"`
		OtherUserID string `json:"otherUserId"`
		Segments    []struct {
			Type       string `json:"type"`
			DurationMs int    `json:"durationMs"`
		} `json:"segments"`
	}
	json.Unmarshal(rr.Body.Bytes(), &sent)
	if sent.Type != "signal" || sent.Status != "completed" || sent.OtherUserID != "user-2" || len(sent.Segments) != 2 {
		t.Fatalf("expected a completed signal send to the real bound target with its real segments, got %+v", sent)
	}

	fakeCore.mu.Lock()
	defer fakeCore.mu.Unlock()
	if len(fakeCore.notificationsSent) != 1 || fakeCore.notificationsSent[0] != "user-2" {
		t.Fatalf("expected exactly one push notification to user-2, got %v", fakeCore.notificationsSent)
	}
}

func TestKnockHandlerRejectsAnInvalidPattern(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, _ := newFakeCoreServer(t, "pulse_friend", "active", true)
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/knocks", strings.NewReader(`{"receiverId":"user-2","pattern":"not_a_real_pattern"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid pattern, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStopBeforeStartReturnsConflict(t *testing.T) {
	svc := pulseinteractions.NewService(memory.New(), &fakeRealtime{}, &fakeAnalytics{}, fakeRateLimiter{allow: true})
	server, _ := newFakeCoreServer(t, "pulse_friend", "active", true)
	mux := http.NewServeMux()
	pulseinteractions.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newPresenceFactory(server.URL), newNotifierFactory("app-1"), fakeSignals{}, fixedCallerWithCore("caller-1", server.URL))

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
