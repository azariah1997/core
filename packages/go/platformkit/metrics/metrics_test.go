package metrics

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMiddlewareRecordsRequestsByRoutePattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware("test-service", mux, mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/users/abc-123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	Handler().ServeHTTP(metricsRec, metricsReq)
	body := metricsRec.Body.String()

	// The route PATTERN, not the literal path (which would carry the
	// user ID and blow up cardinality), must appear in the exposed
	// metrics - this is the whole point of using mux.Handler(r) instead
	// of r.URL.Path.
	if !strings.Contains(body, `path="/v1/users/{id}"`) {
		t.Fatalf("expected metrics labeled with the route pattern, not the literal path; body:\n%s", body)
	}
	if strings.Contains(body, `path="/v1/users/abc-123"`) {
		t.Fatal("metrics must never be labeled with the literal request path (unbounded cardinality)")
	}
}

func TestMiddlewareForwardsHijackForWebSocketUpgrades(t *testing.T) {
	// realtime-gateway's hand-rolled WS handler does w.(http.Hijacker)
	// directly (internal/ws/handler.go) - if this middleware's wrapper
	// doesn't forward Hijack, that type assertion fails and every real
	// WebSocket connection breaks. This is not hypothetical: it broke
	// on the first version of this middleware and was only caught here.
	mux := http.NewServeMux()
	hijacked := false
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter passed through Middleware does not implement http.Hijacker")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Hijack failed: %v", err)
		}
		hijacked = true
		conn.Close()
	})

	handler := Middleware("test-service", mux, mux)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	conn, err := (&net.Dialer{}).Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: test\r\n\r\n"))

	// give the handler a moment to hijack and close its end
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.Read(buf) //nolint:errcheck // just draining until the hijacked conn closes

	if !hijacked {
		t.Fatal("expected the handler to successfully hijack the connection")
	}
}

func TestMiddlewareRecordsUnmatchedRoutesWithoutPanicking(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// A handler for everything else, matching this platform's real
	// router convention (router.go registers "/" as a catch-all).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	handler := Middleware("test-service", mux, mux)
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from the catch-all, got %d", rec.Code)
	}
}

func TestSetDBPoolStatsExposesAllFourStates(t *testing.T) {
	SetDBPoolStats("test-db-service", PoolStats{AcquiredConns: 3, IdleConns: 7, TotalConns: 10, MaxConns: 20})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	body := rec.Body.String()

	for _, want := range []string{
		`db_pool_connections{service="test-db-service",state="acquired"} 3`,
		`db_pool_connections{service="test-db-service",state="idle"} 7`,
		`db_pool_connections{service="test-db-service",state="total"} 10`,
		`db_pool_connections{service="test-db-service",state="max"} 20`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected metrics to contain %q; body:\n%s", want, body)
		}
	}
}

func TestRealtimeConnectionsGaugeIncrementsAndDecrements(t *testing.T) {
	IncRealtimeConnections()
	IncRealtimeConnections()
	DecRealtimeConnections()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, "realtime_ws_connections 1") {
		t.Fatalf("expected realtime_ws_connections to net to 1 after 2 incs and 1 dec; body:\n%s", body)
	}
}

func TestFailureCountersIncrementByLabel(t *testing.T) {
	IncNotificationFailure("push")
	IncNotificationFailure("push")
	IncJobFailure("webhook")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `notification_delivery_failures_total{channel="push"} 2`) {
		t.Fatalf("expected 2 push notification failures; body:\n%s", body)
	}
	if !strings.Contains(body, `job_failures_total{type="webhook"} 1`) {
		t.Fatalf("expected 1 webhook job failure; body:\n%s", body)
	}
}
