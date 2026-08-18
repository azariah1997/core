package messaging_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/messaging"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

func fixedUser(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: userID})))
		})
	}
}

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func createConversation(t *testing.T, mux http.Handler, appID, convType string, memberUserIDs []string) map[string]any {
	t.Helper()
	membersJSON, err := json.Marshal(memberUserIDs)
	if err != nil {
		t.Fatalf("marshal members: %v", err)
	}
	req := httptest.NewRequest("POST", "/v1/conversations",
		strings.NewReader(`{"appId":"`+appID+`","type":"`+convType+`","memberUserIds":`+string(membersJSON)+`}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	return decodeBody(t, rr)
}

func TestCreateConversationEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	svc, _ := newService()
	messaging.RegisterRoutes(mux, svc, fixedUser("u1"))

	body := createConversation(t, mux, "app-1", "group", []string{"u1", "u2"})
	if body["type"] != "group" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestSendAndListMessagesEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	svc, _ := newService()
	messaging.RegisterRoutes(mux, svc, fixedUser("u1"))

	conv := createConversation(t, mux, "app-1", "group", []string{"u1"})
	convID := conv["id"].(string)

	req := httptest.NewRequest("POST", "/v1/conversations/"+convID+"/messages",
		strings.NewReader(`{"type":"TEXT","body":{"text":"hello"}}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	msg := decodeBody(t, rr)
	if msg["type"] != "TEXT" {
		t.Fatalf("unexpected message body: %v", msg)
	}

	listReq := httptest.NewRequest("GET", "/v1/conversations/"+convID+"/messages", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	listBody := decodeBody(t, listRR)
	items, ok := listBody["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 message in list, got %v", listBody)
	}
}

func TestNonMemberCannotSendMessageEndToEnd(t *testing.T) {
	svc, _ := newService()
	ownerMux := http.NewServeMux()
	messaging.RegisterRoutes(ownerMux, svc, fixedUser("u1"))
	conv := createConversation(t, ownerMux, "app-1", "group", []string{"u1"})
	convID := conv["id"].(string)

	strangerMux := http.NewServeMux()
	messaging.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))

	req := httptest.NewRequest("POST", "/v1/conversations/"+convID+"/messages", strings.NewReader(`{"type":"TEXT"}`))
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestReactionEndToEnd(t *testing.T) {
	svc, _ := newService()
	mux := http.NewServeMux()
	messaging.RegisterRoutes(mux, svc, fixedUser("u1"))
	conv := createConversation(t, mux, "app-1", "group", []string{"u1"})
	convID := conv["id"].(string)

	sendReq := httptest.NewRequest("POST", "/v1/conversations/"+convID+"/messages", strings.NewReader(`{"type":"TEXT"}`))
	sendRR := httptest.NewRecorder()
	mux.ServeHTTP(sendRR, sendReq)
	msg := decodeBody(t, sendRR)
	messageID := msg["id"].(string)

	reactReq := httptest.NewRequest("POST", "/v1/conversations/"+convID+"/messages/"+messageID+"/reactions",
		strings.NewReader(`{"type":"👍"}`))
	reactRR := httptest.NewRecorder()
	mux.ServeHTTP(reactRR, reactReq)
	if reactRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", reactRR.Code, reactRR.Body.String())
	}

	removeReq := httptest.NewRequest("DELETE", "/v1/conversations/"+convID+"/messages/"+messageID+"/reactions/%F0%9F%91%8D", nil)
	removeRR := httptest.NewRecorder()
	mux.ServeHTTP(removeRR, removeReq)
	if removeRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", removeRR.Code, removeRR.Body.String())
	}
}

func TestLeaveConversationEndToEnd(t *testing.T) {
	svc, _ := newService()
	ownerMux := http.NewServeMux()
	messaging.RegisterRoutes(ownerMux, svc, fixedUser("u1"))
	conv := createConversation(t, ownerMux, "app-1", "group", []string{"u1", "u2"})
	convID := conv["id"].(string)

	memberMux := http.NewServeMux()
	messaging.RegisterRoutes(memberMux, svc, fixedUser("u2"))

	req := httptest.NewRequest("DELETE", "/v1/conversations/"+convID+"/members/u2", nil)
	rr := httptest.NewRecorder()
	memberMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
