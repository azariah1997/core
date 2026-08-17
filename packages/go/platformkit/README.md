# platformkit

Shared Go foundations used by every backend service (`core-api`, `realtime-gateway`, `worker`). Nothing here is product-specific; it exists so services don't reimplement the same request/observability/config plumbing.

| Package | Responsibility |
|---|---|
| `config` | Typed configuration loaded from environment variables with local-dev defaults; `Validate()` fails fast in `PLATFORM_ENV=production` if required values are missing or still local-development defaults. |
| `logging` | Structured (`log/slog`, JSON) logger tagged with service name and environment. |
| `correlation` | Generates/propagates a correlation ID through request context and the `X-Correlation-ID`/`X-Request-ID` response headers. |
| `apperr` | The platform's single error response shape (`{code, message, correlationId}`) and the HTTP status each `Code` maps to. |
| `health` | `/livez`, `/readyz`, `/healthz` handlers with distinct semantics: liveness never checks dependencies, readiness gates traffic on them, health aggregates both for dashboards without gating. |
| `httpx` | Small HTTP response/method helpers. |
| `runx` | Runs an `http.Server` with SIGINT/SIGTERM-driven graceful shutdown. |
| `otelx` | Initializes the OpenTelemetry trace provider (OTLP/HTTP export to the collector) and wraps handlers for request tracing. |
| `events` | The event envelope shape shared by outbox/Kafka producers. |

## Conventions

- Every service composes: `config.Load()` → `cfg.Validate()` → `otelx.Init` → build handler wrapped in `correlation.Middleware` and `otelx.Wrap` → `runx.Serve`.
- Handlers return errors via `apperr.Write`, never ad hoc maps, so every error response is `{code, message, correlationId}`.
- Avoid adding dependencies here that only one service needs — put those in that service instead.
