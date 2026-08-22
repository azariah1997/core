package livetouch_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/livetouch"
	livetouchcore "github.com/example/core-platform/apps/pulse/api/internal/livetouch/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeCoreServer stands in for both core-api's notifications route and
// realtime-gateway's presence route - one fake server plays both roles
// in tests, matching pulse-interactions' own http_test.go convention.
type fakeCoreServer struct {
	mu                sync.Mutex
	notificationsSent []string
}

func newFakeCoreServer(t *testing.T, online bool) (*httptest.Server, *fakeCoreServer) {
	f := &fakeCoreServer{}
	mux := http.NewServeMux()
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

func newPresenceFactory(realtimeAPIURL string) livetouch.PresenceFactory {
	return func(token string) livetouch.Presence {
		return livetouchcore.NewPresenceAdapter(realtimeAPIURL, token)
	}
}

func newNotifierFactory(pulseAppID string) livetouch.NotifierFactory {
	return func(client *coresdk.Client) livetouch.Notifier {
		return livetouchcore.NewNotifierAdapter(client, pulseAppID)
	}
}

func newRouterFor(callerID, serverURL string, svc *livetouch.Service) *http.ServeMux {
	mux := http.NewServeMux()
	livetouch.RegisterRoutes(mux, svc, newPresenceFactory(serverURL), newNotifierFactory("app-1"), fixedCallerWithCore(callerID, serverURL))
	return mux
}

func TestInviteHandlerRejectsAnUnbondedCaller(t *testing.T) {
	server, _ := newFakeCoreServer(t, true)
	svc := newService(fakeBond{hasBond: false}, &fakeRealtime{}, true)
	mux := newRouterFor("caller-1", server.URL, svc)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFullLiveTouchLifecycleThroughTheRealRouter(t *testing.T) {
	server, fakeCore := newFakeCoreServer(t, true) // receiver online
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	initiatorMux := newRouterFor("caller-1", server.URL, svc)
	receiverMux := newRouterFor("partner-1", server.URL, svc)

	inviteRR := httptest.NewRecorder()
	initiatorMux.ServeHTTP(inviteRR, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions", nil))
	if inviteRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", inviteRR.Code, inviteRR.Body.String())
	}
	var invited struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		DeliveryMode string `json:"deliveryMode"`
	}
	json.Unmarshal(inviteRR.Body.Bytes(), &invited)
	if invited.Status != "invited" || invited.DeliveryMode != "live" {
		t.Fatalf("expected an invited/live session, got %+v", invited)
	}

	acceptRR := httptest.NewRecorder()
	receiverMux.ServeHTTP(acceptRR, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions/"+invited.ID+"/accept", nil))
	if acceptRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on accept, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}
	var accepted struct {
		Status  string `json:"status"`
		Channel string `json:"channel"`
	}
	json.Unmarshal(acceptRR.Body.Bytes(), &accepted)
	if accepted.Status != "active" || !strings.Contains(accepted.Channel, "pulse:live-touch:"+invited.ID) {
		t.Fatalf("expected an active session with a real channel, got %+v", accepted)
	}

	endRR := httptest.NewRecorder()
	initiatorMux.ServeHTTP(endRR, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions/"+invited.ID+"/end", nil))
	if endRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on end, got %d: %s", endRR.Code, endRR.Body.String())
	}
	var ended struct {
		Status     string `json:"status"`
		EndReason  string `json:"endReason"`
		DurationMs int    `json:"durationMs"`
	}
	json.Unmarshal(endRR.Body.Bytes(), &ended)
	if ended.Status != "ended" || ended.EndReason != "ended" {
		t.Fatalf("expected ended/ended, got %+v", ended)
	}

	fakeCore.mu.Lock()
	notified := len(fakeCore.notificationsSent)
	fakeCore.mu.Unlock()
	if notified != 0 {
		t.Fatalf("expected no push notification for a live-delivered invite, got %d", notified)
	}
}

func TestOfflinePartnerGetsAPushInviteThroughTheRealRouter(t *testing.T) {
	server, fakeCore := newFakeCoreServer(t, false) // receiver offline
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	mux := newRouterFor("caller-1", server.URL, svc)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions", nil))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"deliveryMode":"push"`) {
		t.Fatalf("expected push delivery mode for an offline partner, got %s", rr.Body.String())
	}

	fakeCore.mu.Lock()
	defer fakeCore.mu.Unlock()
	if len(fakeCore.notificationsSent) != 1 || fakeCore.notificationsSent[0] != "partner-1" {
		t.Fatalf("expected exactly one push notification to partner-1, got %v", fakeCore.notificationsSent)
	}
}

func TestDeclineHandlerRejectsTheInitiator(t *testing.T) {
	server, _ := newFakeCoreServer(t, true)
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	initiatorMux := newRouterFor("caller-1", server.URL, svc)

	inviteRR := httptest.NewRecorder()
	initiatorMux.ServeHTTP(inviteRR, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions", nil))
	var invited struct {
		ID string `json:"id"`
	}
	json.Unmarshal(inviteRR.Body.Bytes(), &invited)

	rr := httptest.NewRecorder()
	initiatorMux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/live-touch/sessions/"+invited.ID+"/decline", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for the initiator trying to decline their own invite, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetHandlerReturns404ForANonExistentSession(t *testing.T) {
	server, _ := newFakeCoreServer(t, true)
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"}, &fakeRealtime{}, true)
	mux := newRouterFor("caller-1", server.URL, svc)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pulse/live-touch/sessions/does-not-exist", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
