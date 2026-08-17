package correlation

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareGeneratesIDWhenAbsent(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	})
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	Middleware(next).ServeHTTP(rr, req)

	if seen == "" {
		t.Fatal("expected a correlation ID to be set in context")
	}
	if rr.Header().Get(HeaderCorrelationID) != seen {
		t.Fatalf("expected response header %s to match context value", HeaderCorrelationID)
	}
	if rr.Header().Get(HeaderRequestID) != seen {
		t.Fatalf("expected response header %s to match context value", HeaderRequestID)
	}
}

func TestMiddlewarePreservesInboundID(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderCorrelationID, "inbound-id")
	rr := httptest.NewRecorder()
	Middleware(next).ServeHTTP(rr, req)

	if seen != "inbound-id" {
		t.Fatalf("expected inbound correlation ID to be preserved, got %q", seen)
	}
}
