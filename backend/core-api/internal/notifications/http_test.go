package notifications_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
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

func TestSendNotificationEndToEnd(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, nil)
	mux := http.NewServeMux()
	notifications.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/notifications",
		strings.NewReader(`{"appId":"app-1","userId":"u1","category":"message","channels":["in_app"],"title":"Hi","body":"there"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	body := decodeBody(t, rr)
	notif, ok := body["notification"].(map[string]any)
	if !ok || notif["title"] != "Hi" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestSendToSomeoneElseWithoutAdminEndToEnd(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, nil)
	mux := http.NewServeMux()
	notifications.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/notifications",
		strings.NewReader(`{"appId":"app-1","userId":"u2","category":"message","channels":["in_app"],"title":"Hi","body":"there"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListMyNotificationsEndToEnd(t *testing.T) {
	svc := newService(map[notifications.Channel]notifications.ChannelSender{
		notifications.ChannelInApp: &recordingSender{},
	}, nil)
	mux := http.NewServeMux()
	notifications.RegisterRoutes(mux, svc, fixedUser("u1"))

	sendReq := httptest.NewRequest("POST", "/v1/notifications",
		strings.NewReader(`{"appId":"app-1","userId":"u1","category":"message","channels":["in_app"],"title":"Hi","body":"there"}`))
	mux.ServeHTTP(httptest.NewRecorder(), sendReq)

	listReq := httptest.NewRequest("GET", "/v1/notifications", nil)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	body := decodeBody(t, listRR)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 notification in list, got %v", body)
	}
}

func TestPreferenceRoundTripEndToEnd(t *testing.T) {
	svc := newService(nil, nil)
	mux := http.NewServeMux()
	notifications.RegisterRoutes(mux, svc, fixedUser("u1"))

	setReq := httptest.NewRequest("PUT", "/v1/notification-preferences",
		strings.NewReader(`{"appId":"app-1","category":"message","channel":"email","enabled":false}`))
	setRR := httptest.NewRecorder()
	mux.ServeHTTP(setRR, setReq)
	if setRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", setRR.Code, setRR.Body.String())
	}

	getReq := httptest.NewRequest("GET", "/v1/notification-preferences?appId=app-1", nil)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	body := decodeBody(t, getRR)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 preference, got %v", body)
	}
}

func TestTemplateCreateRequiresAdminEndToEnd(t *testing.T) {
	svc := newService(nil, nil)
	mux := http.NewServeMux()
	notifications.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/notification-templates",
		strings.NewReader(`{"appId":"app-1","key":"message.new","titleTemplate":"a","bodyTemplate":"b"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-admin, got %d: %s", rr.Code, rr.Body.String())
	}
}
