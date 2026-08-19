# Observability stack (Phase 28)

"Every production component" here means `core-api`, `realtime-gateway`, and `worker` - each has real structured logs, metrics, distributed traces, and health checks; see `packages/go/platformkit/metrics/README.md` for the metrics-to-dashboard mapping and `packages/go/platformkit/README.md` for the logging/tracing packages.

## Why services push, not get scraped/tailed, for logs and traces

These three services run locally as bare `go run` host processes (`make run-api`/`run-realtime`/`run-worker`), not inside Docker - a log-shipping agent that tails container logs (Promtail) or a sidecar wouldn't see them. So:

- **Traces**: `otelx.Init` pushes spans to `otel-collector` over OTLP/HTTP (`:4318`), which forwards them to Tempo over OTLP/gRPC.
- **Logs**: `logging.NewWithLoki` pushes structured log records directly to Loki's own native push API (`POST /loki/api/v1/push`, `:3100`) - no collector hop needed, since Loki accepts pushes natively. This also sidesteps needing the (still-evolving) OTel Go logs SDK entirely.
- **Metrics**: the one exception - Prometheus *pulls*, scraping each service's real `/metrics` endpoint directly (`prometheus.yml`'s `scrape_configs`), the normal Prometheus model regardless of where the target process runs.

## Files

- `prometheus.yml` - scrape targets (`core-api:8080`, `realtime:8090`, `worker:8091`) and `rule_files: [alerts.yml]`.
- `alerts.yml` - real, live-evaluated Prometheus alerting rules (`ServiceDown`, `HighErrorRate`, `HighAPILatency`, `DBPoolNearExhaustion`, `JobFailuresOccurring`, `NotificationFailuresOccurring`) - queryable at `http://localhost:9090/api/v1/rules` and `/api/v1/alerts`. No Alertmanager service is wired (no real Slack/PagerDuty/email destination exists in this environment to route a firing alert to - the same "no real credentials" reasoning Stripe/AI vendor adapters have documented elsewhere), so a firing alert is real and visible in Prometheus's own API/UI, just not routed anywhere further.
- `loki.yaml` - a real config (not the image's bundled demo default) enabling native OTLP-adjacent structured-metadata support for the push API above.
- `tempo.yaml`, `otel-collector.yaml` - unchanged from earlier phases; traces only, no logs pipeline (logs bypass the collector entirely, see above).
- `grafana/provisioning/datasources/datasources.yml` - Prometheus (default), Tempo, and Loki, wired automatically on container start (no manual "add data source" click-through). Loki's `derivedFields` regex-matches a `"traceId":"..."` field in a log line and turns it into a clickable jump straight to that trace in Tempo - real, but only populated for log lines actually carrying a `traceId` (see the trace-correlation note below).
- `grafana/provisioning/dashboards/dashboards.yml` - points Grafana at `grafana/dashboards/` for auto-loaded dashboard JSON.
- `grafana/dashboards/core-platform-overview.json` - the one dashboard this phase ships, with a real panel per roadmap-named metric (see `packages/go/platformkit/metrics/README.md`'s table) plus an explicit placeholder for Kafka lag.

## Trace-correlated logs: real, but only where a call site opts in

`logging.NewWithLoki`'s handler attaches a real `trace_id`/`span_id` to a log record *if* it was logged through a `*Context` slog method (`InfoContext`, `ErrorContext`, ...) against a context carrying an active span - proven with a real trace context in `packages/go/platformkit/logging/loki_test.go`. Two real call sites (`notifications.Service.dispatch`'s two failure-persistence error logs) were converted to prove the pattern; the other ~50+ existing `logger.Error(...)`/`logger.Info(...)` call sites across 24 modules still use the context-free form and won't carry trace correlation until converted the same way - real, additive, incremental work, not a framework limitation. `packages/go/platformkit/logging/README` (see the package's own doc comments) is the reference for converting more.

## How to look at all of it

```sh
make local-up   # starts prometheus/grafana/loki/tempo/otel-collector alongside everything else
make run-api & make run-realtime & make run-worker &
```

Then:
- Grafana: `http://localhost:3000` (admin/admin) → **Core Platform** folder → **Core Platform Overview**.
- Prometheus: `http://localhost:9090` → Status → Targets (confirm all three are `up`) or Alerts.
- Loki: query via Grafana's Explore, or directly: `curl -G http://localhost:3100/loki/api/v1/query_range --data-urlencode 'query={service="core-api"}'`.
- Tempo: query via Grafana's Explore, or directly: `curl http://localhost:3200/api/search?tags=service.name%3Dcore-api`.

`scripts/smoke.sh` (Phase 28) also asserts every service's `/metrics` genuinely exposes `http_requests_total`, not just that the process answers `/healthz`.
