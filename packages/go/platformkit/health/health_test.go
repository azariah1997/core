package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveAlwaysOK(t *testing.T) {
	rr := httptest.NewRecorder()
	Live("svc")(rr, httptest.NewRequest("GET", "/livez", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
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
	Health("svc", failing)(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz should aggregate not gate, expected 200, got %d", rr.Code)
	}
}
