package mood_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	bondmemory "github.com/example/core-platform/apps/pulse/api/internal/bond/memory"
	"github.com/example/core-platform/apps/pulse/api/internal/mood"
	moodmemory "github.com/example/core-platform/apps/pulse/api/internal/mood/memory"
	"github.com/example/core-platform/apps/pulse/api/internal/mood/pulsemodules"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	pulseconnectionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/core"
	pulseconnectionsmemory "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeRelationshipsServer stands in for core-api's real /v1/relationships
// route - the one Core call mood's own Set indirectly makes, through
// pulse-connections' real *Service and adapter, exactly as production
// wires it (see internal/api/router.go's newMoodConnections).
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
			ctx = pulseauth.WithToken(ctx, "fake-token")
			ctx = pulseauth.WithUser(ctx, coresdk.User{ID: id, Timezone: "UTC"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// newServices builds the real, shared, same-process dependencies mood's
// real router.go wiring uses in production: a real pulse-connections
// *Service and a real bond *Service (both in-memory here), reached
// through internal/mood/pulsemodules' real adapters - never a fake
// mood.Connections/mood.Bond directly, so these tests exercise the real
// cross-module wiring.
func newServices() (*pulseconnections.Service, *mood.Service) {
	connectionsSvc := pulseconnections.NewService(pulseconnectionsmemory.New())
	bondSvc := bond.NewService(bondmemory.New())
	moodSvc := mood.NewService(moodmemory.New(), pulsemodules.NewBondAdapter(bondSvc), &fakeAnalytics{})
	return connectionsSvc, moodSvc
}

func newRouterFor(serverURL, callerID string, connectionsSvc *pulseconnections.Service, moodSvc *mood.Service) *http.ServeMux {
	mux := http.NewServeMux()
	newConnections := func(client *coresdk.Client) mood.Connections {
		return pulsemodules.NewConnectionsAdapter(connectionsSvc, pulseconnectionscore.New(client, "app-1"))
	}
	mood.RegisterRoutes(mux, moodSvc, newConnections, fixedCaller(callerID, serverURL))
	return mux
}

func TestSetHandlerRejectsAnInvalidBody(t *testing.T) {
	server := fakeRelationshipsServer(t, nil)
	connectionsSvc, moodSvc := newServices()
	mux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("PUT", "/v1/pulse/mood", strings.NewReader(`{"emoji":"🚀","audience":"private"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unrecognized emoji, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetAndGetOwnMoodThroughTheRealRouter(t *testing.T) {
	server := fakeRelationshipsServer(t, nil)
	connectionsSvc, moodSvc := newServices()
	mux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)

	setRR := httptest.NewRecorder()
	mux.ServeHTTP(setRR, httptest.NewRequest("PUT", "/v1/pulse/mood", strings.NewReader(`{"emoji":"🔥","audience":"private"}`)))
	if setRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on set, got %d: %s", setRR.Code, setRR.Body.String())
	}
	var set struct {
		Emoji    string `json:"emoji"`
		Audience string `json:"audience"`
	}
	json.Unmarshal(setRR.Body.Bytes(), &set)
	if set.Emoji != "🔥" || set.Audience != "private" {
		t.Fatalf("expected the real emoji/audience echoed back, got %+v", set)
	}

	meRR := httptest.NewRecorder()
	mux.ServeHTTP(meRR, httptest.NewRequest("GET", "/v1/pulse/mood/me", nil))
	if meRR.Code != http.StatusOK || !strings.Contains(meRR.Body.String(), "🔥") {
		t.Fatalf("expected the just-set Mood back from /me, got %d: %s", meRR.Code, meRR.Body.String())
	}
}

func TestGetSomeoneElsesMoodThroughTheRealRouterHonorsAudience(t *testing.T) {
	// caller-1 (the Mood owner) and user-2 are active pulse_friend
	// connections in Core's own real relationship graph, reached
	// through pulse-connections' real *Service via the fake core-api
	// server below.
	server := fakeRelationshipsServer(t, []map[string]any{
		relItem("rel-1", "caller-1", "user-2", "active"),
	})
	connectionsSvc, moodSvc := newServices()
	ownerMux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)
	viewerMux := newRouterFor(server.URL, "user-2", connectionsSvc, moodSvc)
	strangerMux := newRouterFor(server.URL, "stranger", connectionsSvc, moodSvc)

	setRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(setRR, httptest.NewRequest("PUT", "/v1/pulse/mood", strings.NewReader(`{"emoji":"🌊","audience":"all_connections"}`)))
	if setRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on set, got %d: %s", setRR.Code, setRR.Body.String())
	}

	viewRR := httptest.NewRecorder()
	viewerMux.ServeHTTP(viewRR, httptest.NewRequest("GET", "/v1/pulse/mood/caller-1", nil))
	if viewRR.Code != http.StatusOK || !strings.Contains(viewRR.Body.String(), "🌊") {
		t.Fatalf("expected the real active connection to see the Mood, got %d: %s", viewRR.Code, viewRR.Body.String())
	}
	// A non-owner viewer never sees Audience/UpdatedAt/customUserIds -
	// that's the owner's own management detail (toMoodResponse).
	if strings.Contains(viewRR.Body.String(), `"audience"`) {
		t.Fatalf("expected owner-only detail to be omitted from a non-owner's view, got %s", viewRR.Body.String())
	}

	strangerRR := httptest.NewRecorder()
	strangerMux.ServeHTTP(strangerRR, httptest.NewRequest("GET", "/v1/pulse/mood/caller-1", nil))
	if strangerRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-connection outside the audience, got %d: %s", strangerRR.Code, strangerRR.Body.String())
	}
}

func TestGetReturns404ForANonExistentMood(t *testing.T) {
	server := fakeRelationshipsServer(t, nil)
	connectionsSvc, moodSvc := newServices()
	mux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pulse/mood/never-set-user", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestClearHandlerRemovesTheMoodThroughTheRealRouter(t *testing.T) {
	server := fakeRelationshipsServer(t, nil)
	connectionsSvc, moodSvc := newServices()
	mux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/v1/pulse/mood", strings.NewReader(`{"emoji":"💤","audience":"private"}`)))

	clearRR := httptest.NewRecorder()
	mux.ServeHTTP(clearRR, httptest.NewRequest("DELETE", "/v1/pulse/mood", nil))
	if clearRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", clearRR.Code, clearRR.Body.String())
	}

	meRR := httptest.NewRecorder()
	mux.ServeHTTP(meRR, httptest.NewRequest("GET", "/v1/pulse/mood/me", nil))
	if meRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after Clear, got %d: %s", meRR.Code, meRR.Body.String())
	}
}

func TestSetCloseFriendsAudienceThroughTheRealRouterUsesRealClassification(t *testing.T) {
	server := fakeRelationshipsServer(t, []map[string]any{
		relItem("rel-1", "caller-1", "user-2", "active"),
	})
	connectionsSvc, moodSvc := newServices()
	ownerMux := newRouterFor(server.URL, "caller-1", connectionsSvc, moodSvc)
	viewerMux := newRouterFor(server.URL, "user-2", connectionsSvc, moodSvc)

	// Owner classifies user-2 as a close friend through pulse-connections'
	// own real Service - never mood duplicating that data.
	if _, err := connectionsSvc.SetClassification(context.Background(), "caller-1", "rel-1", pulseconnections.SetClassificationInput{Classification: pulseconnections.ClassificationCloseFriend}); err != nil {
		t.Fatalf("set classification: %v", err)
	}

	setRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(setRR, httptest.NewRequest("PUT", "/v1/pulse/mood", strings.NewReader(`{"emoji":"❤️","audience":"close_friends"}`)))
	if setRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on set, got %d: %s", setRR.Code, setRR.Body.String())
	}

	viewRR := httptest.NewRecorder()
	viewerMux.ServeHTTP(viewRR, httptest.NewRequest("GET", "/v1/pulse/mood/caller-1", nil))
	if viewRR.Code != http.StatusOK {
		t.Fatalf("expected the real close friend to see a close_friends Mood, got %d: %s", viewRR.Code, viewRR.Body.String())
	}
}
