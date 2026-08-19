# Events Workflows module

Owns the **events-workflows** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Two packages. `backend/core-api/internal/jobs` owns `Job`/`JobStatus`/`JobAttempt` - immediate/scheduled/delayed/recurring, with retry and dead-letter handling - executed by `worker`, never inline in an HTTP handler; exposed at `/v1/jobs*`. `backend/core-api/internal/workflows` owns the Core Workflow API/SDK, a deliberately narrow abstraction (`Start`/`Describe`/`Signal` only) over a real Temporal cluster for long-running, multi-step, compensating, or scheduled processes - Temporal itself is never exposed directly to a caller; exposed at `/v1/workflows*`. Privacy (Phase 20) additionally runs its own embedded Temporal worker inside `core-api`'s own process, since `worker` is a separate Go module that cannot import `core-api`'s internal packages. See each package's README for detail.
