package pulseconnections_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	pulseconnectionscore "github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/core"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections/memory"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeCoreServer is a real local HTTP server standing in for core-api's
// relationships routes - the same real-local-server testing convention
// every SDK in this platform already uses, rather than a mocked
// interface, since internal/pulseconnections/core (the real adapter
// under test here) talks to Core purely over HTTP.
type fakeCoreServer struct {
	mu        sync.Mutex
	rels      map[string]map[string]any
	groups    map[string]map[string]any
	members   map[string][]map[string]any // groupID -> members
	next      int
	nextGroup int
}

func newFakeCoreServer(t *testing.T) *httptest.Server {
	f := &fakeCoreServer{rels: map[string]map[string]any{}, groups: map[string]map[string]any{}, members: map[string][]map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AppID        string `json:"appId"`
			TargetUserID string `json:"targetUserId"`
			Type         string `json:"type"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.next++
		id := fmt.Sprintf("rel-%d", f.next)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
		rel := map[string]any{
			"id": id, "requesterUserId": "caller-1", "targetUserId": body.TargetUserID,
			"status": "pending", "createdAt": now, "updatedAt": now,
		}
		f.rels[id] = rel
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("POST /v1/relationships/{id}/accept", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		rel, ok := f.rels[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"code": "RESOURCE_NOT_FOUND", "message": "not found"})
			return
		}
		rel["status"] = "active"
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("POST /v1/relationships/{id}/decline", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		rel, ok := f.rels[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"code": "RESOURCE_NOT_FOUND", "message": "not found"})
			return
		}
		rel["status"] = "ended"
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("DELETE /v1/relationships/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.rels, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		items := make([]map[string]any, 0, len(f.rels))
		for _, rel := range f.rels {
			items = append(items, rel)
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	mux.HandleFunc("POST /v1/groups", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AppID string `json:"appId"`
			Name  string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.nextGroup++
		id := fmt.Sprintf("circle-%d", f.nextGroup)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
		g := map[string]any{"id": id, "appId": body.AppID, "name": body.Name, "status": "active", "createdAt": now, "updatedAt": now}
		f.groups[id] = g
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(g)
	})
	mux.HandleFunc("GET /v1/groups", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		items := make([]map[string]any, 0, len(f.groups))
		for _, g := range f.groups {
			items = append(items, g)
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	mux.HandleFunc("GET /v1/groups/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"items": f.members[r.PathValue("id")]})
	})
	mux.HandleFunc("POST /v1/groups/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserID string `json:"userId"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
		m := map[string]any{"userId": body.UserID, "role": "", "isManager": false, "createdAt": now}
		f.members[r.PathValue("id")] = append(f.members[r.PathValue("id")], m)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("DELETE /v1/groups/{id}/members/{userId}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var kept []map[string]any
		for _, m := range f.members[r.PathValue("id")] {
			if m["userId"] != r.PathValue("userId") {
				kept = append(kept, m)
			}
		}
		f.members[r.PathValue("id")] = kept
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func fixedCallerWithCore(t *testing.T, id string) func(http.Handler) http.Handler {
	server := newFakeCoreServer(t)
	client := coresdk.NewClient(server.URL, coresdk.WithTokenSource(coresdk.StaticTokenSource("fake-token")), coresdk.WithRetries(1, 0))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := pulseauth.WithCaller(r.Context(), id)
			ctx = pulseauth.WithClient(ctx, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newCoreFactory(pulseAppID string) pulseconnections.CoreFactory {
	return func(client *coresdk.Client) pulseconnections.CoreRelationships {
		return pulseconnectionscore.New(client, pulseAppID)
	}
}

func newGroupsFactory(pulseAppID string) pulseconnections.GroupsFactory {
	return func(client *coresdk.Client) pulseconnections.CoreGroups {
		return pulseconnectionscore.NewGroupsAdapter(client, pulseAppID)
	}
}

func TestRequestConnectionHandlerRejectsAMissingTarget(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	req := httptest.NewRequest("POST", "/v1/pulse/connections/requests", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRequestAndListConnectionsRoundTripThroughTheRealRouterAndAFakeCoreServer(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	createReq := httptest.NewRequest("POST", "/v1/pulse/connections/requests", strings.NewReader(`{"targetUserId":"user-2"}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	if !strings.Contains(createRR.Body.String(), `"classification":"friend"`) {
		t.Fatalf("expected the default friend classification, got %s", createRR.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/v1/pulse/connections", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "user-2") {
		t.Fatalf("expected the requested connection in the list, got %s", listRR.Body.String())
	}
}

func TestSetClassificationHandlerRejectsAnInvalidValue(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	req := httptest.NewRequest("PATCH", "/v1/pulse/connections/rel-1", strings.NewReader(`{"classification":"nonsense"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAddCircleMemberRequiresAnExistingConnection(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, httptest.NewRequest("POST", "/v1/pulse/circles", strings.NewReader(`{"name":"Family"}`)))
	var circle struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &circle)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/circles/"+circle.ID+"/members", strings.NewReader(`{"userId":"stranger"}`)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for adding a non-connection to a Circle, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCircleMembershipRoundTripsForARealConnectionThroughTheRealRouter(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	createConnRR := httptest.NewRecorder()
	mux.ServeHTTP(createConnRR, httptest.NewRequest("POST", "/v1/pulse/connections/requests", strings.NewReader(`{"targetUserId":"user-2"}`)))
	var conn struct {
		RelationshipID string `json:"relationshipId"`
	}
	json.Unmarshal(createConnRR.Body.Bytes(), &conn)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/pulse/connections/requests/"+conn.RelationshipID+"/accept", nil))

	createCircleRR := httptest.NewRecorder()
	mux.ServeHTTP(createCircleRR, httptest.NewRequest("POST", "/v1/pulse/circles", strings.NewReader(`{"name":"Family"}`)))
	if createCircleRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating a Circle, got %d: %s", createCircleRR.Code, createCircleRR.Body.String())
	}
	var circle struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createCircleRR.Body.Bytes(), &circle)

	addRR := httptest.NewRecorder()
	mux.ServeHTTP(addRR, httptest.NewRequest("POST", "/v1/pulse/circles/"+circle.ID+"/members", strings.NewReader(`{"userId":"user-2"}`)))
	if addRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 adding a real connection as a Circle member, got %d: %s", addRR.Code, addRR.Body.String())
	}

	listMembersRR := httptest.NewRecorder()
	mux.ServeHTTP(listMembersRR, httptest.NewRequest("GET", "/v1/pulse/circles/"+circle.ID+"/members", nil))
	if listMembersRR.Code != http.StatusOK || !strings.Contains(listMembersRR.Body.String(), "user-2") {
		t.Fatalf("expected user-2 in the Circle's member list, got %d: %s", listMembersRR.Code, listMembersRR.Body.String())
	}

	listCirclesRR := httptest.NewRecorder()
	mux.ServeHTTP(listCirclesRR, httptest.NewRequest("GET", "/v1/pulse/circles", nil))
	if listCirclesRR.Code != http.StatusOK || !strings.Contains(listCirclesRR.Body.String(), "Family") {
		t.Fatalf("expected Family in my Circles, got %d: %s", listCirclesRR.Code, listCirclesRR.Body.String())
	}

	removeRR := httptest.NewRecorder()
	mux.ServeHTTP(removeRR, httptest.NewRequest("DELETE", "/v1/pulse/circles/"+circle.ID+"/members/user-2", nil))
	if removeRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204 removing a Circle member, got %d: %s", removeRR.Code, removeRR.Body.String())
	}
}

func TestAcceptHandlerForwardsARealNotFoundFromCore(t *testing.T) {
	svc := pulseconnections.NewService(memory.New())
	mux := http.NewServeMux()
	pulseconnections.RegisterRoutes(mux, svc, newCoreFactory("app-1"), newGroupsFactory("app-1"), fixedCallerWithCore(t, "caller-1"))

	req := httptest.NewRequest("POST", "/v1/pulse/connections/requests/does-not-exist/accept", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 forwarded from the fake core server, got %d: %s", rr.Code, rr.Body.String())
	}
}
