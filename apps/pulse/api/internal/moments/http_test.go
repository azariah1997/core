package moments_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/moments"
	"github.com/example/core-platform/apps/pulse/api/internal/moments/memory"
	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
)

func fixedCaller(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := pulseauth.WithCaller(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newRouterFor(callerID string, svc *moments.Service, interactions moments.PulseInteractions) *http.ServeMux {
	mux := http.NewServeMux()
	moments.RegisterRoutes(mux, svc, interactions, fixedCaller(callerID))
	return mux
}

func TestSaveHandlerRejectsANonParticipant(t *testing.T) {
	svc := moments.NewService(memory.New())
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed", Type: "pulse", CreatedAt: time.Now().UTC()})
	mux := newRouterFor("stranger", svc, interactions)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/moments/i-1/save", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSaveHandlerRejectsAnIncompleteInteraction(t *testing.T) {
	svc := moments.NewService(memory.New())
	interactions := newFakeInteractions()
	interactions.add(moments.InteractionRef{ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "started"})
	mux := newRouterFor("user-a", svc, interactions)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/pulse/moments/i-1/save", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSaveListDeleteRoundTripThroughTheRealRouter(t *testing.T) {
	svc := moments.NewService(memory.New())
	interactions := newFakeInteractions()
	durationMs := 1200
	interactions.add(moments.InteractionRef{
		ID: "i-1", SenderID: "user-a", ReceiverID: "user-b", Status: "completed",
		Type: "pulse", DurationMs: &durationMs, CreatedAt: time.Now().UTC(),
	})
	mux := newRouterFor("user-a", svc, interactions)

	saveRR := httptest.NewRecorder()
	mux.ServeHTTP(saveRR, httptest.NewRequest("POST", "/v1/pulse/moments/i-1/save", nil))
	if saveRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", saveRR.Code, saveRR.Body.String())
	}
	var saved struct {
		ID              string `json:"id"`
		OtherUserID     string `json:"otherUserId"`
		InteractionType string `json:"interactionType"`
		DurationMs      int    `json:"durationMs"`
	}
	json.Unmarshal(saveRR.Body.Bytes(), &saved)
	if saved.OtherUserID != "user-b" || saved.InteractionType != "pulse" || saved.DurationMs != 1200 {
		t.Fatalf("expected the real interaction details echoed back, got %+v", saved)
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/pulse/moments", nil))
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), saved.ID) {
		t.Fatalf("expected the saved moment in the list, got %d: %s", listRR.Code, listRR.Body.String())
	}

	deleteRR := httptest.NewRecorder()
	mux.ServeHTTP(deleteRR, httptest.NewRequest("DELETE", "/v1/pulse/moments/"+saved.ID, nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}

	listAfterDeleteRR := httptest.NewRecorder()
	mux.ServeHTTP(listAfterDeleteRR, httptest.NewRequest("GET", "/v1/pulse/moments", nil))
	if strings.Contains(listAfterDeleteRR.Body.String(), saved.ID) {
		t.Fatalf("expected the moment to be gone after delete, got %s", listAfterDeleteRR.Body.String())
	}
}
