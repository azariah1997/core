package pulseprefs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
	pulseprefscore "github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeCoreServer stands in for core-api's real quiet-hours/mutes
// endpoints - enough to prove pulseprefs' real adapters (pulseprefscore)
// wire the real request/response shapes correctly, without needing a
// live Core instance for a handler-layer unit test.
func fakeCoreServer(t *testing.T) *httptest.Server {
	var quietHours map[string]any
	muted := map[string]map[string]any{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/notification-preferences/quiet-hours", func(w http.ResponseWriter, r *http.Request) {
		if quietHours == nil {
			quietHours = map[string]any{"timezone": "UTC", "startMinute": 0, "endMinute": 0, "enabled": false}
		}
		json.NewEncoder(w).Encode(quietHours)
	})
	mux.HandleFunc("PUT /v1/notification-preferences/quiet-hours", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		quietHours = map[string]any{"timezone": body["timezone"], "startMinute": body["startMinute"], "endMinute": body["endMinute"], "enabled": body["enabled"]}
		json.NewEncoder(w).Encode(quietHours)
	})
	mux.HandleFunc("POST /v1/trustsafety/mutes", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mutedUserID := body["mutedUserId"].(string)
		m := map[string]any{"id": "mute-" + mutedUserID, "mutedUserId": mutedUserID, "createdAt": "2026-01-01T00:00:00.000Z"}
		muted[mutedUserID] = m
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("GET /v1/trustsafety/mutes", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{}
		for _, m := range muted {
			items = append(items, m)
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	mux.HandleFunc("DELETE /v1/trustsafety/mutes/{userId}", func(w http.ResponseWriter, r *http.Request) {
		delete(muted, r.PathValue("userId"))
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
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

func newRouterFor(callerID, serverURL string, svc *pulseprefs.Service) *http.ServeMux {
	mux := http.NewServeMux()
	newQuietHours := func(client *coresdk.Client) pulseprefs.CoreQuietHours {
		return pulseprefscore.NewQuietHoursAdapter(client, "app-1")
	}
	newMutes := func(client *coresdk.Client) pulseprefs.CoreMutes {
		return pulseprefscore.NewMutesAdapter(client)
	}
	pulseprefs.RegisterRoutes(mux, svc, newQuietHours, newMutes, fixedCaller(callerID, serverURL))
	return mux
}

func TestPreferencesGetDefaultsThenSetRoundTrips(t *testing.T) {
	server := fakeCoreServer(t)
	mux := newRouterFor("caller-1", server.URL, pulseprefs.NewService(memory.New()))

	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest("GET", "/v1/pulse/preferences", nil))
	if getRR.Code != http.StatusOK || !strings.Contains(getRR.Body.String(), `"notificationDetail":"detailed"`) {
		t.Fatalf("expected the real default preferences, got %d: %s", getRR.Code, getRR.Body.String())
	}

	setRR := httptest.NewRecorder()
	mux.ServeHTTP(setRR, httptest.NewRequest("PUT", "/v1/pulse/preferences", strings.NewReader(`{"notificationDetail":"private","hapticIntensity":0.6}`)))
	if setRR.Code != http.StatusOK || !strings.Contains(setRR.Body.String(), `"notificationDetail":"private"`) {
		t.Fatalf("expected the real updated preferences, got %d: %s", setRR.Code, setRR.Body.String())
	}

	getAfterRR := httptest.NewRecorder()
	mux.ServeHTTP(getAfterRR, httptest.NewRequest("GET", "/v1/pulse/preferences", nil))
	if !strings.Contains(getAfterRR.Body.String(), `"notificationDetail":"private"`) {
		t.Fatalf("expected the persisted preferences on re-read, got %s", getAfterRR.Body.String())
	}
}

func TestSetPreferencesHandlerRejectsAnInvalidNotificationDetail(t *testing.T) {
	server := fakeCoreServer(t)
	mux := newRouterFor("caller-1", server.URL, pulseprefs.NewService(memory.New()))

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("PUT", "/v1/pulse/preferences", strings.NewReader(`{"notificationDetail":"loud","hapticIntensity":0.5}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestQuietHoursHandlerRoundTripsThroughTheRealCoreAdapter(t *testing.T) {
	server := fakeCoreServer(t)
	mux := newRouterFor("caller-1", server.URL, pulseprefs.NewService(memory.New()))

	setRR := httptest.NewRecorder()
	mux.ServeHTTP(setRR, httptest.NewRequest("PUT", "/v1/pulse/preferences/quiet-hours", strings.NewReader(`{"timezone":"America/New_York","startMinute":1320,"endMinute":420,"enabled":true}`)))
	if setRR.Code != http.StatusOK || !strings.Contains(setRR.Body.String(), `"timezone":"America/New_York"`) {
		t.Fatalf("expected the real quiet hours echoed back, got %d: %s", setRR.Code, setRR.Body.String())
	}

	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest("GET", "/v1/pulse/preferences/quiet-hours", nil))
	if !strings.Contains(getRR.Body.String(), `"enabled":true`) {
		t.Fatalf("expected the real Core-stored quiet hours on read, got %s", getRR.Body.String())
	}
}

func TestMuteListUnmuteHandlersRoundTripThroughTheRealCoreAdapter(t *testing.T) {
	server := fakeCoreServer(t)
	mux := newRouterFor("caller-1", server.URL, pulseprefs.NewService(memory.New()))

	muteRR := httptest.NewRecorder()
	mux.ServeHTTP(muteRR, httptest.NewRequest("POST", "/v1/pulse/preferences/mutes", strings.NewReader(`{"mutedUserId":"user-b"}`)))
	if muteRR.Code != http.StatusCreated || !strings.Contains(muteRR.Body.String(), `"mutedUserId":"user-b"`) {
		t.Fatalf("expected the real mute created, got %d: %s", muteRR.Code, muteRR.Body.String())
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/pulse/preferences/mutes", nil))
	if !strings.Contains(listRR.Body.String(), "user-b") {
		t.Fatalf("expected the real muted user in the list, got %s", listRR.Body.String())
	}

	deleteRR := httptest.NewRecorder()
	mux.ServeHTTP(deleteRR, httptest.NewRequest("DELETE", "/v1/pulse/preferences/mutes/user-b", nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}

	listAfterRR := httptest.NewRecorder()
	mux.ServeHTTP(listAfterRR, httptest.NewRequest("GET", "/v1/pulse/preferences/mutes", nil))
	if strings.Contains(listAfterRR.Body.String(), "user-b") {
		t.Fatalf("expected the mute to be gone after unmute, got %s", listAfterRR.Body.String())
	}
}
