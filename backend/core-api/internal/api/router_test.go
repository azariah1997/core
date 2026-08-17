package api

import (
	"github.com/example/core-platform/packages/go/platformkit/config"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestGenericDataAPIIsBlocked(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/data/query", nil)
	rr := httptest.NewRecorder()
	New(config.Load()).ServeHTTP(rr, req)
	if rr.Code != 501 {
		t.Fatalf("expected 501, got %d", rr.Code)
	}
}
