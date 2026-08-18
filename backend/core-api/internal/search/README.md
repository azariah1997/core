# Search Platform

The platform's single search entry point - "applications should not access OpenSearch directly" is the roadmap's own framing. `Service.Query` is what callers use; OpenSearch itself is reached only through `platformkit/searchidx.Provider`, never directly.

## Responsibilities

- `SearchDocument` (`searchidx.Document`): a generic, product-defined `Type` (`"user"`, `"message"`, `"application"`, or anything a future domain module registers - never a fixed platform enum, the same convention as `RelationshipType`/`Message.Type`), an optional `AppID` (some entities, like platform Users, aren't scoped to one application), and a free-form `Fields` map.
- `SearchProvider` (`searchidx.Provider`): the abstraction over the actual engine. `OpenSearchProvider` is the only implementation today, but the interface is what makes swapping engines later a new `Provider`, not a rewrite of every caller.
- `Query` is open to any authenticated caller, filtered by the optional `Type`/`AppID` the caller supplies.
- `Index`/`Delete` are the manual, on-demand re-indexing path, platform.admin only. The *automatic* path is event-driven, per the roadmap's own example ("`user.updated` → search index worker") - see `backend/worker/internal/indexer`, which talks to `searchidx.Provider` directly rather than through this HTTP-facing `Service`.

## Scoping decisions

- **No per-document visibility enforcement.** Search only ever surfaces what's already been indexed; whether the querying caller is actually allowed to see a *specific* result (e.g., a private message) is not checked here. Enforcing that properly would mean this package re-implementing every other domain module's access rules for every document type it might ever index - out of scope for a generic platform capability. Filtering by `Type`/`AppID` is the only scoping this phase provides; a product needing per-document visibility would need to layer it on top (e.g., by never indexing content the product doesn't want globally searchable in the first place).
- **Indexed documents are thin**, containing only what the triggering outbox event's payload already carries (a handful of key fields), not full entity content. `worker` is a separate Go module from `core-api` and can't import its internal domain packages to re-read a full record, and there's no service-to-service auth infrastructure yet to fetch one over HTTP instead. A deliberate, documented scope-down - see `backend/worker/internal/indexer`'s own doc comment for the full reasoning.
- **One shared OpenSearch index** (`searchidx.DefaultIndex`) for every document type and app, distinguished by the `type`/`appId` fields on each document, rather than one index per type. Simpler to operate when this phase doesn't yet know how many document types future domain modules will ever register.

## Layout

- `domain.go` - `QueryInput`/`IndexInput` DTOs (this package's own contract, translated to/from `searchidx` types) and validation.
- `service.go` - `Service`, the `AdminChecker` interface it depends on (satisfied directly by `*authz.Service`, no adapter needed).
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.

The shared `searchidx.Document`/`Provider` contract and the real `OpenSearchProvider` live in `packages/go/platformkit/searchidx` - both this package (querying) and `worker`'s indexer (writing) depend on the exact same types, the same reason `platformkit/rtbus` exists for realtime pub/sub: two services acting on the same data can't each define their own compatible-by-luck shape.
