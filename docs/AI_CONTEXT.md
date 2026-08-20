# AI Context Contract

Read this file before modifying the repository. It reflects the real architecture as built across all 30 roadmap phases (see `VALIDATION.md` for the full phase-by-phase history) - not aspirational rules written before the code existed.

## Non-negotiable rules

- Core modules never import product-specific code.
- Do not add a generic SQL-over-HTTP API.
- Do not bypass authorization at service boundaries - every permission check goes through `authz.Service.Can`/`IsPlatformAdmin` (`backend/core-api/internal/authz`), never a module inventing its own logic.
- Do not add a new infrastructure product when an existing platform capability covers the requirement.
- Any persistent schema change requires a migration (`data/migrations/NNNN_*.sql`).
- Any public API change updates OpenAPI (`contracts/openapi/core-api.yaml`) - operationIds are module-prefixed (`groupsGet`, not `get`).
- Any event change updates AsyncAPI (`contracts/asyncapi/events.yaml`) and increments the event version if incompatible.
- Any cross-service RPC change updates Protobuf.
- Add/adjust tests with behavior changes - both a service-layer test (in-memory fakes) and a handler-layer test (real `http.ServeMux` + `RegisterRoutes`), see "Testing" below.
- Update relevant ADR when changing an accepted architecture decision (`docs/decisions/ADR-*.md`).
- Update `catalog/system.yaml` when a module's dependencies or API/event surface changes - Backstage's catalog is the real dependency graph, not documentation prose describing one.
- Never commit credentials, signing keys, tokens or personal data. Never fabricate a vendor integration against credentials that don't exist in this environment - if a real provider (Stripe, Apple IAP, OpenAI) has no real credentials here, say so explicitly and leave it structurally supported but unimplemented, the same honest-gap pattern this repo has used since Phase 22.

## The module pattern

Every domain module under `backend/core-api/internal/<module>/` follows the same layered shape:

```
domain.go            # types + interfaces the service depends on (consumer-defined, not provider-defined)
service.go           # business rules, authorization checks, validation - the only thing HTTP handlers call
http.go               # RegisterRoutes(mux, svc, requireUser) + handlers; translates HTTP <-> service calls only
postgres/repository.go  # production storage, implements domain.go's repository interface
memory/repository.go    # in-memory fake, same interface, used by every service_test.go
README.md            # Responsibilities / Non-responsibilities / Layout / Storage - the real architecture notes
```

Interfaces are defined by the consumer (`service.go`), not the provider - `postgres/` and `memory/` both satisfy whatever `domain.go` declares, so swapping storage or writing a test never touches `service.go`. Modules with genuinely circular dependencies (e.g. `privacy` needing to call `users`/`devices`/`files`/`audit` to export/delete a person's data, while those modules must never import `privacy`) resolve it with two-phase wiring: `service.go` exposes `RegisterExporter`/`RegisterDeleter` registries, and `internal/api/wiring.go` calls them after all services are constructed - dependencies point inward at startup, never at compile time.

`RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler)` is the shared signature across every module. The caller is always resolved via `users.FromContext(r.Context())` inside a private per-module `callerID` helper - never trusted from a request body field. Ownership/authorization gating (who may call what) lives in `service.go`, not `http.go` - the HTTP layer is a thin translation, so a caller hitting the service directly (e.g. from a Temporal workflow) gets the same guarantees as one going through HTTP.

## Testing

Every module has two layers of tests:

- **Service tests** (`service_test.go`, package `<module>_test`): exercise business rules against the in-memory repository/dependencies (`memory.New()` + local `fake*` types implementing whatever `domain.go` declares). This is where validation rules, ownership checks, and state transitions are proven.
- **Handler tests** (`http_test.go`, package `<module>_test`, added Phase 30 to close a real gap - 12 of 22 modules had zero handler-level coverage until then): exercise the real `http.NewServeMux()` + `RegisterRoutes` + `mux.ServeHTTP`, proving request/response JSON shapes and HTTP status codes against real source, not assumed ones. Every file defines its own local `fixedUser(id string) func(http.Handler) http.Handler` (attaches `users.WithUser` to the request context) rather than sharing one across files - this codebase prefers a little repetition per test file over cross-file coupling.

There is deliberately no `postgres/`-level Go test file in any module. Repository-layer correctness against real Postgres is proven by live validation during each phase's own rollout (see `VALIDATION.md`), not mocked - this is a considered choice, not a gap: mocking a SQL driver would prove the mock behaves correctly, not that the real query does.

## Real-infrastructure-only validation

No phase in this repository has ever claimed success from a mock. Every "Validate" step in the phase loop below ran against a genuinely running Postgres, Valkey, Keycloak, OpenFGA, MinIO, OpenSearch, Temporal, Ollama, or Prometheus/Grafana/Loki/Tempo - never a stand-in. When a real vendor has no real credentials in this environment (Stripe live keys, Apple IAP/Google Play sandboxes, OpenAI/Anthropic/Google API keys), the honest response has been to leave the integration structurally supported (a real interface, a real adapter shape) but not implemented, and to say so explicitly in `VALIDATION.md` - never to fabricate a passing test against a mock standing in for the vendor. Where a real vendor protocol could be exercised without a live account (Stripe's webhook HMAC signature scheme, Ollama's local OpenAI-compatible inference), it was - self-signed test payloads and a real auto-pulled local model, not a `nock`-style intercept.

## The map of the platform

`catalog/system.yaml` (Backstage, Phase 26) is the real, machine-checked map: every service, domain module, and app as a `Component`, with `dependsOn` mirroring each module's actual code dependencies and `providesApis`/`consumesApis` set only where a module's code genuinely calls `outbox.Record` (cross-checked against source, never assumed). `docs/control/platform.json` (Phase 29) is a second, machine-generated view of the same underlying data (roadmap status, modules, catalog components, AsyncAPI events) - both come from one generation script (`docs/control/build_workbook.py`) and a fresh parse of the same source files, so the two views cannot drift apart the way two independently hand-maintained documents eventually would.

## Client interfaces

Products consume the platform through one of three official SDKs (Phase 27) - `packages/go/coresdk`, `packages/typescript/core-sdk`, `packages/flutter/core_sdk` - not by hand-rolling HTTP calls. All three share one design (a `Client`/`CoreClient` core, `TokenSource` for auth/refresh, GET-only safe retries, cursor pagination over the platform's real `{items, nextCursor}` shape, a realtime WebSocket client matching `realtime-gateway`'s actual wire protocol) translated idiomatically per language. `apps/admin` and `apps/mobile` are real consumers, not just SDK test harnesses - if an SDK's shape doesn't match what a real app needs, that's a signal the SDK is wrong, not the app.

## Observability

Every service exposes real `/metrics` (Prometheus, route-pattern-labeled via `mux.Handler(r)` to keep cardinality bounded - never the raw request path), ships structured logs to Loki with `trace_id`/`span_id` correlation when logging against a context carrying an active OTel span, and has carried distributed traces (Tempo) and `/livez`/`/readyz`/`/healthz` since Phase 1. `packages/go/platformkit/metrics`, `logging`, and `otelx` are the shared implementations - a module should never hand-roll its own metric registration or log correlation.

## The phase loop

Every phase in this repository's history followed the same loop, stated explicitly because deviating from it is the most common way an AI agent's work here goes wrong: **Inspect** the real current state before assuming what exists (`grep`/read the actual source, never guess a function signature or a field name) → **Design** the smallest real change that closes the actual gap → **Implement** → **Test** (service-layer and handler-layer, see "Testing") → **Validate live** against real infrastructure, never a mock, and record what was found (including real bugs, fixed honestly) → **Document** (`VALIDATION.md` entry, README/ADR/catalog updates as needed) → **Commit**, conventionally as two commits: `feat(<area>): Phase N - <Name>` for the change itself, then `docs(control): update dashboard through Phase N` for the workbook/`platform.json` regeneration.

## Change checklist

1. Identify the owning module (check `catalog/system.yaml` and `modules/*/module.yaml` if unsure of boundaries).
2. Review the module's real dependencies (`module.yaml`, and `domain.go`'s interfaces - not just what's documented).
3. Change contract first when applicable (OpenAPI/AsyncAPI/Protobuf).
4. Implement, following the layered module pattern above.
5. Test: a service-layer test and, if `http.go` changed, a handler-layer test.
6. Validate live against real infrastructure - `make smoke`, direct `curl`/SDK calls, or a real browser session, never a mock standing in for the real thing.
7. Update docs/catalog - the module README, `catalog/system.yaml` if dependencies/APIs/events changed, `VALIDATION.md`, and this file if a fundamental architecture rule changed.
8. Submit PR; never deploy production directly.
