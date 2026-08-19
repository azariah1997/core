package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiveAlwaysOK(t *testing.T) {
	rr := httptest.NewRecorder()
	Live("svc", "abc1234")(rr, httptest.NewRequest("GET", "/livez", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"version":"abc1234"`) {
		t.Fatalf("expected the real version to appear in the response, got %s", rr.Body.String())
	}
}

func TestReadyReflectsFailingDependency(t *testing.T) {
	failing := func(ctx context.Context) []Result {
		return []Result{{Name: "db", OK: false, Error: "boom"}}
	}
	rr := httptest.NewRecorder()
	Ready(failing)(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestReadyOKWithNoChecks(t *testing.T) {
	rr := httptest.NewRecorder()
	Ready(nil)(rr, httptest.NewRequest("GET", "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHealthAlwaysReturns200EvenWhenNotReady(t *testing.T) {
	failing := func(ctx context.Context) []Result {
		return []Result{{Name: "db", OK: false, Error: "boom"}}
	}
	rr := httptest.NewRecorder()
	Health("svc", "abc1234", failing)(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz should aggregate not gate, expected 200, got %d", rr.Code)
	}
}
