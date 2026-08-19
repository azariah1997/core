// Package metrics gives every Go service in this platform a real
// Prometheus /metrics endpoint and a small set of shared instruments
// (HTTP request count/duration/in-flight, DB pool stats, realtime
// WebSocket connections, Redis command latency, notification/job
// failures) - the concrete metrics the roadmap's own dashboard list
// names (API latency, error rate, requests/sec, DB connections,
// WebSocket connections, Redis latency, notification failures,
// background job failures). Kafka lag is deliberately not here - no
// real Kafka/Redpanda producer or consumer exists in this codebase yet
// (see infra/observability/grafana/dashboards's own placeholder panel).
package metrics

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests, labeled by the matched route pattern (never a raw path, to keep cardinality bounded).",
	}, []string{"service", "method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds, labeled by the matched route pattern.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "path"})

	httpRequestsInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	}, []string{"service"})

	dbPoolConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_pool_connections",
		Help: "pgxpool connection pool state (acquired/idle/total/max).",
	}, []string{"service", "state"})

	realtimeWSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "realtime_ws_connections",
		Help: "Currently active WebSocket connections on realtime-gateway.",
	})

	redisCommandDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "redis_command_duration_seconds",
		Help:    "Valkey/Redis command duration in seconds, labeled by command name.",
		Buckets: prometheus.DefBuckets,
	}, []string{"command"})

	notificationDeliveryFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_delivery_failures_total",
		Help: "Notification deliveries that ended in a failed status, labeled by channel.",
	}, []string{"channel"})

	jobFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "job_failures_total",
		Help: "Jobs that exhausted their retries and moved to dead_letter, labeled by job type.",
	}, []string{"type"})
)

// Handler serves the real Prometheus exposition format - register it at
// GET /metrics in each service's router, the same convention as
// /livez /readyz /healthz.
func Handler() http.Handler {
	return promhttp.Handler()
}

// muxHandler is the subset of *http.ServeMux this package needs -
// looking up the pattern a request WOULD match without serving it, so
// HTTP metrics are labeled by route pattern ("GET /v1/users/{id}"),
// never a raw path (which would blow up cardinality with one time
// series per literal user ID).
type muxHandler interface {
	Handler(r *http.Request) (h http.Handler, pattern string)
}

// Middleware records http_requests_total/http_request_duration_seconds/
// http_requests_in_flight for every request mux would route, then
// serves it via next (the real handler chain - typically the same mux,
// further wrapped in CORS/correlation/auth middleware).
func Middleware(service string, mux muxHandler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern == "" {
			pattern = "unmatched"
		}
		// http.ServeMux.Handler returns the pattern verbatim as
		// registered, which for a method-specific route (this
		// platform's convention - "GET /v1/users/{id}") includes the
		// method as a literal prefix ("GET /v1/users/{id}"). Strip it -
		// the method is already its own label, and leaving it in would
		// make every "path" value redundant with "method" for no reason.
		if sp := strings.IndexByte(pattern, ' '); sp != -1 && !strings.ContainsAny(pattern[:sp], "/{") {
			pattern = pattern[sp+1:]
		}

		inFlight := httpRequestsInFlight.WithLabelValues(service)
		inFlight.Inc()
		defer inFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		httpRequestsTotal.WithLabelValues(service, r.Method, pattern, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(service, r.Method, pattern).Observe(duration)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Hijack forwards to the underlying ResponseWriter's own Hijacker -
// required for this middleware to sit in front of realtime-gateway's
// hand-rolled WebSocket upgrade (internal/ws/handler.go calls
// w.(http.Hijacker) directly), which would otherwise fail its own type
// assertion against this wrapper and break every WebSocket connection.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("metrics: underlying ResponseWriter does not support http.Hijacker")
	}
	return hj.Hijack()
}

// Flush forwards to the underlying ResponseWriter's own Flusher, for
// any handler (SSE, chunked streaming) that needs to flush before the
// response completes - the same "don't silently break the interfaces
// the wrapped handler relies on" reasoning as Hijack above.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// PoolStats is the subset of *pgxpool.Pool.Stat()'s real fields this
// package needs - a plain struct instead of importing jackc/pgx
// directly, so this package doesn't force every caller into a specific
// Postgres driver.
type PoolStats struct {
	AcquiredConns int32
	IdleConns     int32
	TotalConns    int32
	MaxConns      int32
}

// SetDBPoolStats records a real Postgres connection pool snapshot -
// call it on a periodic ticker (e.g. every 10s) from each service that
// holds a *pgxpool.Pool.
func SetDBPoolStats(service string, stats PoolStats) {
	dbPoolConnections.WithLabelValues(service, "acquired").Set(float64(stats.AcquiredConns))
	dbPoolConnections.WithLabelValues(service, "idle").Set(float64(stats.IdleConns))
	dbPoolConnections.WithLabelValues(service, "total").Set(float64(stats.TotalConns))
	dbPoolConnections.WithLabelValues(service, "max").Set(float64(stats.MaxConns))
}

// IncRealtimeConnections/DecRealtimeConnections track realtime-gateway's
// live WebSocket connection count - call from the hub's Register/
// Unregister, the two points where the platform actually knows a
// connection started or ended.
func IncRealtimeConnections() { realtimeWSConnections.Inc() }
func DecRealtimeConnections() { realtimeWSConnections.Dec() }

// IncNotificationFailure records a notification delivery that ended
// failed - call from the exact point notifications/service.go's
// dispatch already decides StatusFailed, not from a separate poller.
func IncNotificationFailure(channel string) {
	notificationDeliveryFailures.WithLabelValues(channel).Inc()
}

// IncJobFailure records a job that exhausted its retries and moved to
// dead_letter - call from worker/internal/jobrunner's dead-letter
// transition specifically, not every individual retryable attempt
// failure (which is expected, routine behavior, not an alertable one).
func IncJobFailure(jobType string) {
	jobFailures.WithLabelValues(jobType).Inc()
}

// redisHook implements redis.Hook (go-redis v9) to time every real
// command without touching call sites in ratelimit/presence/hub -
// InstrumentRedis is called once, where each service constructs its
// *redis.Client.
type redisHook struct{}

func (redisHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (redisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		redisCommandDuration.WithLabelValues(cmd.Name()).Observe(time.Since(start).Seconds())
		return err
	}
}

func (redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		elapsed := time.Since(start).Seconds()
		for _, cmd := range cmds {
			redisCommandDuration.WithLabelValues(cmd.Name()).Observe(elapsed)
		}
		return err
	}
}

// InstrumentRedis adds a real go-redis v9 hook to client that observes
// redis_command_duration_seconds for every command the client issues,
// regardless of which package (ratelimit, presence, hub) issues it -
// call once right after constructing the client.
func InstrumentRedis(client *redis.Client) {
	client.AddHook(redisHook{})
}
