# Backstage (Phase 26 - developer portal)

A real, scaffolded Backstage instance (`@backstage/create-app`, not hand-rolled) whose software catalog is this platform's real metadata - every backend module, service, and app, sourced from `catalog/system.yaml` at the repo root (plus `contracts/openapi/`, `contracts/asyncapi/`, each carrying their own colocated `catalog-info.yaml`). Backstage becomes the map of the platform, per the roadmap's own framing for this phase - not a second, hand-maintained copy of what `docs/control`'s dashboard already tracks operationally.

## Running it locally

Backstage's toolchain needs a real Yarn install and a Node version it supports (this repo's other JS work, `apps/admin`, only needs whatever `node`/`npm` is already on `PATH` - Backstage is stricter). If the system `node` is too new for Backstage's prerequisite check:

```sh
brew install node@22 yarn   # keg-only, doesn't touch the system `node`
export PATH="$(brew --prefix node@22)/bin:$(brew --prefix)/bin:$PATH"
cd platform/backstage
yarn install
yarn start
```

Then open `http://localhost:3000` - the catalog is the landing page. The backend listens on `:7007`; local dev storage is an in-memory SQLite database, so the catalog is rebuilt from the real `catalog/*.yaml` files fresh on every restart (consistent with this repo's every other local-dev-storage service - Keycloak, OpenFGA - also self-healing from a static import on restart).

## Why the catalog isn't declared entirely in `catalog/system.yaml`

The two API entities (`core-api`'s OpenAPI spec, the platform's AsyncAPI event contract) live in `contracts/openapi/catalog-info.yaml` and `contracts/asyncapi/catalog-info.yaml` instead, each embedding its spec file's full content directly in `spec.definition` rather than referencing it with a `$text: ../contracts/...` placeholder. That's a real, live-confirmed limitation of this Backstage version, not a stylistic choice: a `type: file` catalog location's `baseUrl` is a bare filesystem path, and the `$text` resolver's `new URL(value, baseUrl)` call requires `baseUrl` to have a URL scheme - it throws `TypeError: Invalid URL` for every local file location, same-directory references included (confirmed by testing both a same-directory and a parent-relative `$text` ref; both failed identically). `scripts/embed-catalog-api-defs.py` keeps the embedded copies in sync with the real spec files - re-run it (then `scripts/validate_contracts.py`) after editing either spec.

## What's real here vs. deferred

Every Component, System, API, and Group in the catalog describes something that actually exists - no placeholder services, no fake ownership. `dependsOn`/`providesApis`/`consumesApis` mirror `modules/*/module.yaml`'s dependency graph and the real outbox-event publishers (see `catalog/system.yaml`'s own header comment for the granularity decision). Authentication uses Backstage's `guest` provider only - real SSO (e.g. wiring the same Keycloak realm every other service in this platform uses) is a reasonable follow-up, not implemented here since this phase's roadmap scope is "every module exposes metadata," not "stand up a production-hardened internal tool." TechDocs, the software-template scaffolder, and the Kubernetes plugin are present (scaffold defaults) but unconfigured - none of them have real backing data in this environment yet.
