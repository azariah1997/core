# metrics

Phase 28 - "ensure every production component has ... metrics" and "create standard dashboards for API latency, error rate, requests/sec, DB connections, Kafka lag, WebSocket connections, Redis latency, notification failures, background job failures." This package gives every Go service a real Prometheus `/metrics` endpoint and provides all eight of those dashboards with genuine backing data.

## Mapping the roadmap's dashboard list to real metrics

| Dashboard | Metric | Wired from |
|---|---|---|
| API latency | `http_request_duration_seconds` (histogram) | `Middleware`, every service |
| error rate | `http_requests_total{status=~"5.."}` | `Middleware`, every service |
| requests/sec | `http_requests_total` | `Middleware`, every service |
| DB connections | `db_pool_connections` | `pg.ReportStats`, core-api/realtime-gateway/worker |
| WebSocket connections | `realtime_ws_connections` | `IncRealtimeConnections`/`DecRealtimeConnections`, `hub.Register`/`Unregister` |
| Redis latency | `redis_command_duration_seconds` | `InstrumentRedis` (a real go-redis v9 `Hook`), core-api/realtime-gateway |
| notification failures | `notification_delivery_failures_total` | `IncNotificationFailure`, `notifications.Service.dispatch`'s two real failure paths |
| background job failures | `job_failures_total` | `IncJobFailure`, `jobrunner.Runner.recordResult`'s dead-letter transition only (a retryable attempt failure is expected, routine behavior, not this metric) |
| **Kafka lag** | **none** | **Deliberately not implemented - no real Kafka/Redpanda producer or consumer exists in this codebase yet** (documented since Phase 14; Redpanda is health-checked only). The Grafana dashboard has a placeholder panel explaining this instead of a fabricated metric with no real backing data. |

## Route-pattern labeling, not raw paths

`Middleware` calls `mux.Handler(r)` - a real `*http.ServeMux` method that looks up which pattern a request *would* match without serving it - to label `http_requests_total`/`http_request_duration_seconds` by the registered pattern (`"GET /v1/users/{id}"`, stripped of its method prefix since that's already a separate label) rather than the literal request path. Labeling by literal path would create one time series per literal user ID ever requested - unbounded cardinality that would eventually take Prometheus down. `TestMiddlewareRecordsRequestsByRoutePattern` asserts on this directly.

## A real regression this package's own tests caught

`Middleware`'s `statusRecorder` wraps `http.ResponseWriter` to capture the response status code - the standard way to build HTTP metrics middleware. The first version didn't implement `http.Hijacker`, and realtime-gateway's hand-rolled WebSocket handler (`internal/ws/handler.go`) does `w.(http.Hijacker)` directly to take over the raw connection for the WS protocol upgrade. Wrapping `mux` in `Middleware` without forwarding `Hijack` would have broken *every* WebSocket connection in production - caught by `TestMiddlewareForwardsHijackForWebSocketUpgrades` (which dials a real TCP connection against a real `httptest.Server`, not a mocked hijack), and re-confirmed live afterward: a real `coresdk.RealtimeClient` connection through the actual running realtime-gateway, with `realtime_ws_connections` observed going 0 → 1 → 0 across connect/hold/disconnect.

## Layout

- `metrics.go` - the Prometheus collectors, `Handler`, `Middleware`, `statusRecorder` (with `Hijack`/`Flush` forwarding).
- `metrics_test.go` - route-pattern labeling, the Hijack regression test, DB pool stats exposition, the realtime gauge, the failure counters.

## Not done here

Per-service Grafana dashboards (today there's one shared `Core Platform Overview` dashboard, not nine). Alertmanager routing (Prometheus's own alerting *rules* are real and live-evaluated - see `infra/observability/alerts.yml` - but there's no real Slack/PagerDuty/email destination in this environment to route a firing alert to, the same "no real credentials" reasoning Stripe/AI vendor adapters have documented in earlier phases).
