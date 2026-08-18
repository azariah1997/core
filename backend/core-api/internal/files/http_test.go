package files_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/files"
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

func TestRequestUploadEndToEnd(t *testing.T) {
	svc := newService(newFakeStore(), nil)
	mux := http.NewServeMux()
	files.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/files",
		strings.NewReader(`{"appId":"app-1","fileName":"photo.png","mimeType":"image/png","sizeBytes":1024}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	body := decodeBody(t, rr)
	if body["uploadUrl"] == "" || body["uploadUrl"] == nil {
		t.Fatalf("expected an uploadUrl, got %v", body)
	}
	f, ok := body["file"].(map[string]any)
	if !ok || f["status"] != "pending" {
		t.Fatalf("unexpected file body: %v", body)
	}
}

func TestFullUploadLifecycleEndToEnd(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, nil)
	mux := http.NewServeMux()
	files.RegisterRoutes(mux, svc, fixedUser("u1"))

	createReq := httptest.NewRequest("POST", "/v1/files",
		strings.NewReader(`{"appId":"app-1","fileName":"doc.pdf","mimeType":"application/pdf","sizeBytes":100}`))
	createRR := httptest.NewRecorder()
	mux.ServeHTTP(createRR, createReq)
	created := decodeBody(t, createRR)
	fileID := created["file"].(map[string]any)["id"].(string)
	objectKey := "" // fetch via Get to read the real object key isn't exposed in response; simulate upload keyed by re-deriving is unnecessary - use store directly via Get endpoint instead

	getReq := httptest.NewRequest("GET", "/v1/files/"+fileID, nil)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	_ = decodeBody(t, getRR)

	// Simulate the client's direct PUT to storage by finding the object key
	// through the service layer (tests share the same *fakeStore instance).
	f, err := svc.Get(getReq.Context(), "u1", fileID)
	if err != nil {
		t.Fatalf("get file via service: %v", err)
	}
	objectKey = f.ObjectKey
	store.put(objectKey, 100, "checksum")

	confirmReq := httptest.NewRequest("POST", "/v1/files/"+fileID+"/confirm", nil)
	confirmRR := httptest.NewRecorder()
	mux.ServeHTTP(confirmRR, confirmReq)
	if confirmRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", confirmRR.Code, confirmRR.Body.String())
	}
	confirmed := decodeBody(t, confirmRR)
	if confirmed["status"] != "active" {
		t.Fatalf("expected active status after confirm, got %v", confirmed)
	}

	downloadReq := httptest.NewRequest("GET", "/v1/files/"+fileID+"/download", nil)
	downloadRR := httptest.NewRecorder()
	mux.ServeHTTP(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", downloadRR.Code, downloadRR.Body.String())
	}
	downloadBody := decodeBody(t, downloadRR)
	if downloadBody["downloadUrl"] == "" || downloadBody["downloadUrl"] == nil {
		t.Fatalf("expected a downloadUrl, got %v", downloadBody)
	}

	deleteReq := httptest.NewRequest("DELETE", "/v1/files/"+fileID, nil)
	deleteRR := httptest.NewRecorder()
	mux.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}
}

func TestStrangerCannotAccessPrivateFileEndToEnd(t *testing.T) {
	svc := newService(newFakeStore(), nil)
	ownerMux := http.NewServeMux()
	files.RegisterRoutes(ownerMux, svc, fixedUser("u1"))

	createReq := httptest.NewRequest("POST", "/v1/files",
		strings.NewReader(`{"appId":"app-1","fileName":"secret.pdf","mimeType":"application/pdf","sizeBytes":100}`))
	createRR := httptest.NewRecorder()
	ownerMux.ServeHTTP(createRR, createReq)
	created := decodeBody(t, createRR)
	fileID := created["file"].(map[string]any)["id"].(string)

	strangerMux := http.NewServeMux()
	files.RegisterRoutes(strangerMux, svc, fixedUser("stranger"))
	req := httptest.NewRequest("GET", "/v1/files/"+fileID, nil)
	rr := httptest.NewRecorder()
	strangerMux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOversizedUploadRejectedEndToEnd(t *testing.T) {
	svc := files.NewService(nil, newFakeStore(), fakeAdmin{}, files.Config{MaxSizeBytes: 10})
	mux := http.NewServeMux()
	files.RegisterRoutes(mux, svc, fixedUser("u1"))

	req := httptest.NewRequest("POST", "/v1/files",
		strings.NewReader(`{"appId":"app-1","fileName":"big.zip","mimeType":"application/zip","sizeBytes":1000}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
