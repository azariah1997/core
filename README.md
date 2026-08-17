# Core Platform Boilerplate

A vendor-neutral, AI-friendly monorepo for building multiple future applications on top of shared platform capabilities.

## Design goals

- Git is the source of truth.
- Product apps depend on platform contracts, not infrastructure vendors.
- Clear module ownership and docs-as-code for AI-assisted development.
- Modular monolith first; extract services when scale justifies it.
- Local development is one command when Docker is available.
- Production deployment uses Terraform + Kubernetes + Helm + Argo CD.

## Included

- Go Core API, Realtime Gateway and Worker
- Flutter mobile shell + reusable Core SDK scaffold
- Next.js admin/control-plane shell
- Backstage catalog metadata and configuration scaffold
- PostgreSQL migrations with transactional outbox
- Valkey/Redis-compatible cache and presence
- Kafka-compatible event streaming (Redpanda locally)
- Keycloak identity provider
- OpenFGA authorization engine
- Temporal workflow engine
- MinIO local S3-compatible object storage
- OpenSearch search engine
- OpenTelemetry, Prometheus, Grafana, Loki and Tempo observability
- OpenAPI, Protobuf and AsyncAPI contracts
- Docker Compose local stack
- Helm chart and Argo CD application
- Terraform AWS/Cloudflare scaffolding
- GitHub Actions CI
- ADRs, AI context and ownership metadata

## Quick start

```bash
cp .env.example .env
make doctor
make build
make test
make local-up
make smoke
```

`make local-up` requires Docker Desktop. Flutter, Terraform, kubectl and Helm targets require those tools to be installed.

## Repository map

```text
apps/                 Product/admin/mobile shells
backend/              Deployable Go processes
modules/              Platform domain contracts and documentation
packages/              Shared SDKs and libraries
contracts/             OpenAPI / Protobuf / AsyncAPI
platform/              Backstage and control-plane metadata
data/                  Migrations, schemas and seeds
infra/                 Docker, Kubernetes, Terraform, observability
scripts/               Bootstrap, validation and smoke tests
docs/                  Architecture and ADRs
.github/workflows/      CI/CD
```

## Architecture rule

**Core never imports a product. Products consume Core through APIs/SDKs/contracts.**

## Production notes

The repository is deployable infrastructure-as-code, but cloud deployment requires your own AWS/Cloudflare credentials, DNS zone, state backend, signing identities and secrets. Do not commit secrets.
