package devices_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/devices"
	"github.com/example/core-platform/backend/core-api/internal/devices/memory"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

// fixedUser stands in for the api package's real requireUser middleware,
// attaching a known user directly so these tests exercise the HTTP
// handlers independent of identity/users composition.
func fixedUser(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), users.User{ID: id})))
		})
	}
}

func TestRegisterDeviceOmitsRawPushTokenFromResponse(t *testing.T) {
	svc := devices.NewService(memory.New())
	mux := http.NewServeMux()
	devices.RegisterRoutes(mux, svc, fixedUser("user-1"))

	req := httptest.NewRequest("POST", "/v1/users/me/devices",
		strings.NewReader(`{"clientDeviceId":"install-1","platform":"ios","pushToken":"super-secret"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "super-secret") {
		t.Fatal("response body must never contain the raw push token")
	}
	var body map[string]any
	if err := json.NewDecoder(strings.NewReader(rr.Body.String())).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hasPushToken"] != true {
		t.Fatalf("expected hasPushToken true, got %v", body)
	}
}

func TestRegisterHandlerRejectsMissingPlatform(t *testing.T) {
	svc := devices.NewService(memory.New())
	mux := http.NewServeMux()
	devices.RegisterRoutes(mux, svc, fixedUser("user-1"))

	req := httptest.NewRequest("POST", "/v1/users/me/devices", strings.NewReader(`{"clientDeviceId":"install-1"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListReturnsRegisteredDevices(t *testing.T) {
	svc := devices.NewService(memory.New())
	mux := http.NewServeMux()
	devices.RegisterRoutes(mux, svc, fixedUser("user-1"))

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/users/me/devices",
		strings.NewReader(`{"clientDeviceId":"install-1","platform":"ios"}`)))

	req := httptest.NewRequest("GET", "/v1/users/me/devices", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 device, got %d", len(body.Items))
	}
}

func TestDeleteRevokesDeviceAndRemovesFromList(t *testing.T) {
	svc := devices.NewService(memory.New())
	mux := http.NewServeMux()
	devices.RegisterRoutes(mux, svc, fixedUser("user-1"))

	registerRR := httptest.NewRecorder()
	mux.ServeHTTP(registerRR, httptest.NewRequest("POST", "/v1/users/me/devices",
		strings.NewReader(`{"clientDeviceId":"install-1","platform":"ios"}`)))
	var created map[string]any
	_ = json.NewDecoder(registerRR.Body).Decode(&created)
	id := created["id"].(string)

	deleteReq := httptest.NewRequest("DELETE", "/v1/users/me/devices/"+id, nil)
	deleteRR := httptest.NewRecorder()
	mux.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}

	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, httptest.NewRequest("GET", "/v1/users/me/devices", nil))
	var body struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(listRR.Body).Decode(&body)
	if len(body.Items) != 0 {
		t.Fatalf("expected the revoked device to disappear from the list, got %d items", len(body.Items))
	}
}

func TestDeleteUnknownDeviceReturns404(t *testing.T) {
	svc := devices.NewService(memory.New())
	mux := http.NewServeMux()
	devices.RegisterRoutes(mux, svc, fixedUser("user-1"))

	req := httptest.NewRequest("DELETE", "/v1/users/me/devices/11111111-1111-1111-1111-111111111111", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}
