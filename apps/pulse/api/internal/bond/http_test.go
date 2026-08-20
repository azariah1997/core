package bond_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/bond"
	bondcore "github.com/example/core-platform/apps/pulse/api/internal/bond/core"
	"github.com/example/core-platform/apps/pulse/api/internal/bond/memory"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
)

// fakeCoreServer is a real local HTTP server standing in for core-api's
// relationships routes - same convention as pulseconnections' own
// http_test.go. friends lets a test seed a pre-existing active
// pulse_friend connection (required before a bond can be requested).
type fakeCoreServer struct {
	mu      sync.Mutex
	rels    map[string]map[string]any
	friends []map[string]any
	next    int
}

func newFakeCoreServer(t *testing.T, friends []map[string]any) *httptest.Server {
	f := &fakeCoreServer{rels: map[string]map[string]any{}, friends: friends}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/relationships", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TargetUserID string `json:"targetUserId"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.next++
		id := fmt.Sprintf("bond-rel-%d", f.next)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
		rel := map[string]any{"id": id, "requesterUserId": "caller-1", "targetUserId": body.TargetUserID, "status": "pending", "createdAt": now, "updatedAt": now}
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
		rel := f.rels[r.PathValue("id")]
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
		if r.URL.Query().Get("type") == bond.FriendRelationshipType {
			json.NewEncoder(w).Encode(map[string]any{"items": f.friends})
			return
		}
		items := make([]map[string]any, 0, len(f.rels))
		for _, rel := range f.rels {
			items = append(items, rel)
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// fixedCallerWithCore wires the caller's context against a real
// *coresdk.Client pointed at serverURL - a real local server (real
// core-api in production, a fakeCoreServer in tests) that every caller
// in a given test must share, the same way every real user in
// production talks to the one real core-api instance.
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

func newCoreFactory(pulseAppID string) bond.CoreFactory {
	return func(client *coresdk.Client) bond.CoreRelationships {
		return bondcore.New(client, pulseAppID)
	}
}

func existingFriendship(userA, userB string) []map[string]any {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	return []map[string]any{{"id": "friend-1", "requesterUserId": userA, "targetUserId": userB, "status": "active", "createdAt": now, "updatedAt": now}}
}

func TestRequestBondHandlerRejectsWithoutAnExistingConnection(t *testing.T) {
	svc := bond.NewService(memory.New())
	server := newFakeCoreServer(t, nil)
	mux := http.NewServeMux()
	bond.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	req := httptest.NewRequest("POST", "/v1/pulse/bond/requests", strings.NewReader(`{"targetUserId":"user-2"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "connection must exist") {
		t.Fatalf("expected the no-connection message, got %s", rr.Body.String())
	}
}

func TestRequestAndAcceptBondRoundTripThroughTheRealRouterAndAFakeCoreServer(t *testing.T) {
	svc := bond.NewService(memory.New())
	server := newFakeCoreServer(t, existingFriendship("caller-1", "user-2"))
	mux := http.NewServeMux()
	bond.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	createReq := httptest.NewRequest("POST", "/v1/pulse/bond/requests", strings.NewReader(`{"targetUserId":"user-2"}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRR.Body.Bytes(), &created)

	targetMux := http.NewServeMux()
	bond.RegisterRoutes(targetMux, svc, newCoreFactory("app-1"), fixedCallerWithCore("user-2", server.URL))
	acceptReq := httptest.NewRequest("POST", "/v1/pulse/bond/requests/"+created.ID+"/accept", nil)
	acceptRR := httptest.NewRecorder()
	targetMux.ServeHTTP(acceptRR, acceptReq)

	if acceptRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", acceptRR.Code, acceptRR.Body.String())
	}
	if !strings.Contains(acceptRR.Body.String(), `"status":"active"`) {
		t.Fatalf("expected active status, got %s", acceptRR.Body.String())
	}
}

func TestGetMyActiveBondHandlerReturns404WhenUnbonded(t *testing.T) {
	svc := bond.NewService(memory.New())
	server := newFakeCoreServer(t, nil)
	mux := http.NewServeMux()
	bond.RegisterRoutes(mux, svc, newCoreFactory("app-1"), fixedCallerWithCore("caller-1", server.URL))

	req := httptest.NewRequest("GET", "/v1/pulse/bond", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
