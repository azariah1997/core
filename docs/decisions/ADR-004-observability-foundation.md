# ADR-004: OpenTelemetry as the tracing foundation
Status: Accepted

All backend services adopt the OpenTelemetry Go SDK, exporting traces via OTLP/HTTP to the collector already defined in `infra/observability/otel-collector.yaml`. Services never talk to a tracing backend (Tempo, a vendor APM) directly; only to the collector, so the backend can change without touching service code. Tracing initialisation is a no-op when no collector endpoint is configured, so it never blocks local development or tests that don't set one.
