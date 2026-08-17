# Validation Report

Generated validation for this boilerplate in the creation environment.

## Passed
- Repository/config required-artifact checks
- Secret-pattern sanity check
- OpenAPI/AsyncAPI/Protobuf contract presence checks
- SQL migration/outbox static check
- Argo CD and Helm scaffold static check
- YAML parsing for Compose, observability, Argo, Backstage catalog and API contracts
- `gofmt`
- `go test` across all Go workspace modules
- `go build` across all Go workspace modules
- Live Core API health and platform endpoints
- Live Realtime Gateway health endpoint
- Live RFC6455 WebSocket upgrade handshake
- Live Worker health endpoint

## Not executable in the creation environment
The environment used to generate this repo does not contain Docker, Flutter, Terraform, kubectl or Helm and has no AWS/Cloudflare/App Store/Google Play credentials. Therefore these commands are provided but were not falsely marked as executed:

- `make local-up`
- `make flutter-get && make flutter-test`
- `make terraform-validate`
- `make helm-lint`
- real AWS/Kubernetes/Cloudflare deployment
- iOS/Android signing and store deployment

Run `make doctor` on the target Mac after `scripts/bootstrap-macos.sh` to confirm prerequisites.

## Important scope
This is a production-oriented **boilerplate/foundation**, not a completed hosted platform. Infrastructure components, contracts, module boundaries and deploy paths are wired and documented. Domain implementations such as full Keycloak login flows, OpenFGA policies, PostgreSQL repositories, Kafka producers/consumers, billing providers and notification providers are intentionally extension points for the platform modules rather than fake implementations.

## 2026-08-17: Phase 1 Foundation Audit

Ran on a real Mac dev environment (Docker, Flutter, Terraform, kubectl, Helm all installed), so the commands the creation environment could only provide, not execute, were actually run here:

### Passed
- `make doctor`, `gofmt -l` (clean), `go vet` (per module), `go build`, `go test` — all services and `platformkit`
- `python3 scripts/validate_config.py`, `python3 scripts/validate_contracts.py`
- `terraform -chdir=infra/terraform validate`
- `terraform -chdir=infra/terraform fmt -recursive -check`
- `helm lint infra/kubernetes/charts/core-platform`
- `make local-up` — all 13 containers healthy
- `make flutter-get`; Next.js admin `npm install && npm run build`
- Live smoke tests (`make smoke`) against real `core-api`/`realtime-gateway`/`worker` processes
- Manual raw WebSocket handshake against `realtime-gateway` after wrapping its handler in `otelhttp` middleware (confirms `http.Hijacker` still passes through)
- End-to-end OpenTelemetry trace export verified against the real collector/Tempo containers (not just build-time compilation)

### Fixed during this audit
- `infra/terraform/versions.tf`: alignment fix so `terraform fmt -check` (run in CI) passes
- `infra/docker/docker-compose.yml`: `temporal` depended on `postgres` starting, not being ready — added `condition: service_healthy` to prevent a schema-setup crash race
- `infra/observability/tempo.yaml` and `infra/observability/otel-collector.yaml`: OTLP receivers defaulted to binding `localhost` inside their containers, making them unreachable from other containers on the Docker network (and unreliable over IPv6 loopback even from the host) — explicitly bound both to `0.0.0.0`

## 2026-08-17: Phase 2 - Application Registry

First real domain module: `Application` CRUD (`POST/GET/PATCH /v1/apps`, `GET /v1/apps/{id}`) with cursor pagination, slug validation/uniqueness, and `application.created`/`application.updated` events written transactionally to `outbox_events` alongside the row change (verified live: both the row and its outbox event land in the same Postgres transaction).

Validated live against the real `docker-compose` Postgres, not just the in-memory fake used by unit tests: create, get, list with two-page cursor pagination, patch, duplicate-slug 409, invalid-slug 400, unknown-id 404, and the outbox rows themselves (`SELECT * FROM outbox_events`) all confirmed by hand. `data/migrations/0002_applications_registry.sql` adds the `updated_at` column the domain model needs.

Environment/ApplicationVersion/ApplicationConfiguration (also named in the Phase 2 roadmap) are intentionally deferred - documented as such in `backend/core-api/internal/applications/README.md` - since no consumer needs them yet and the roadmap itself only specifies concrete endpoints for `Application`.

### Known gap, not fixed (out of scope for this pass)
- `apps/mobile` has no `test/` directory, so `make flutter-test` fails; not currently wired into CI either. Deferred to when the mobile shell gets real screens worth testing.
- `apps/admin`'s `next@15`/`sharp` pulls in 3 high-severity transitive advisories (postcss, libvips); the fix is a Next.js major-version bump, which is a separate, reviewable change.
- Real PostgreSQL/Redis/Kafka client wiring (as opposed to the TCP-reachability health checks that exist today) is deferred to whichever domain module first needs it, per this file's existing scope note above.
