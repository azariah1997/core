# platformkit

Shared Go foundations used by every backend service (`core-api`, `realtime-gateway`, `worker`). Nothing here is product-specific; it exists so services don't reimplement the same request/observability/config plumbing.

| Package | Responsibility |
|---|---|
| `config` | Typed configuration loaded from environment variables with local-dev defaults; `Validate()` fails fast in `PLATFORM_ENV=production` if required values are missing or still local-development defaults. |
| `logging` | Structured (`log/slog`, JSON) logger tagged with service name and environment. `NewWithLoki` (Phase 28) additionally ships every record to Loki's real push API, attaching the real `trace_id`/`span_id` when logged via a `*Context` slog method against a context carrying an active OTel span. |
| `metrics` (Phase 28) | A real Prometheus `/metrics` endpoint and shared instruments - HTTP request count/duration/in-flight (labeled by route *pattern*, never a raw path), Postgres pool stats, realtime WebSocket connections, Redis command latency, notification/job failure counters. See its own README for the roadmap's full "observability completion" mapping. |
| `correlation` | Generates/propagates a correlation ID through request context and the `X-Correlation-ID`/`X-Request-ID` response headers. |
| `apperr` | The platform's single error response shape (`{code, message, correlationId}`) and the HTTP status each `Code` maps to. |
| `health` | `/livez`, `/readyz`, `/healthz` handlers with distinct semantics: liveness never checks dependencies, readiness gates traffic on them, health aggregates both for dashboards without gating. |
| `httpx` | Small HTTP response/method helpers. |
| `runx` | Runs an `http.Server` with SIGINT/SIGTERM-driven graceful shutdown. |
| `otelx` | Initializes the OpenTelemetry trace provider (OTLP/HTTP export to the collector) and wraps handlers for request tracing. |
| `pg` | The platform's single way to obtain a `*pgxpool.Pool`; `ReportStats` (Phase 28) polls it into `metrics.SetDBPoolStats` on a ticker. |
| `events` | The event envelope shape shared by outbox/Kafka producers. |

## Conventions

- Every service composes: `config.Load()` → `cfg.Validate()` → `otelx.Init` → build handler wrapped in `metrics.Middleware`, `correlation.Middleware`, and `otelx.Wrap` → `runx.Serve`. `metrics.Middleware` wraps the *outermost* handler chain but needs the concrete `*http.ServeMux` too (to label metrics by route pattern via `mux.Handler(r)` without executing it) - see any service's `router.go`/`main.go` for the exact composition.
- Handlers return errors via `apperr.Write`, never ad hoc maps, so every error response is `{code, message, correlationId}`.
- Avoid adding dependencies here that only one service needs — put those in that service instead.
