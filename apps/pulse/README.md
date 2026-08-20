# Pulse

A global, non-verbal social communication product built as a **consumer** of the Core Platform, never a modification of it.

> Feel it instead of reading it.

Pulse lets people who care about each other communicate presence, emotion, and attention without typing or speaking - starting with **Pulse** (press-and-hold, felt as haptic on the other end), extending to **Mood**, **Knock**, **Live Touch**, and a private **Custom Touch Language** between two people.

The full product specification lives in [`docs/PRODUCT_SPEC.md`](docs/PRODUCT_SPEC.md) (106 sections - vision, product model, phased roadmap, definitions of done, principles). The architecture audit required before any implementation began lives in [`docs/ARCHITECTURE_AUDIT.md`](docs/ARCHITECTURE_AUDIT.md). The live-tracked roadmap/todo/user-flow dashboard is [`docs/control/pulse.html`](../../docs/control/pulse.html) at the repo root.

## Dependency direction

```text
Pulse
  ↓
Core Platform
```

Never the reverse. Pulse consumes Core's identity, relationships, groups, realtime, notifications, files, jobs, workflows, feature flags, remote config, audit, privacy, trust & safety, billing, analytics, and observability through the real Go/TypeScript/Dart SDKs and REST/event contracts - it does not reimplement any of them, and Core never acquires a Pulse-specific concept.

## Layout (matches Core's own conventions)

```text
apps/pulse/
  mobile/     Flutter app (apps/mobile's sibling) - the only Pulse UI surface for now
  api/        Pulse Product API (new Go service) - owns Pulse-specific data, calls Core via coresdk
  modules/    Pulse domain module boundaries (module.yaml + README.md, mirrors Core's modules/*/module.yaml)
  contracts/  Pulse-owned OpenAPI/AsyncAPI (Pulse's own API/events, distinct from Core's contracts/)
  docs/       Product spec, architecture audit, phase-by-phase validation as it happens
```

## Status

Phases 1-3 complete and live-validated: Product Foundation, Connection Experience, and Partner Bond. Pulse is registered as a real Core `Application`; `apps/pulse/api` and `apps/pulse/mobile` both exist and run; `pulse-profile` and `pulse-connections` work end to end against real infrastructure (see their own module READMEs). `bond` requires an existing active connection before a Bond can be requested (product spec §11), and enforces the one-active-Bond-per-user invariant with a dedicated Postgres table (`bond_active_holders`, `user_id` as its `PRIMARY KEY`) rather than an application-level check - proven race-safe with a real concurrency test: two goroutines racing `Accept` against real Postgres for the same target user, exactly one succeeding across every run. See `docs/ARCHITECTURE_AUDIT.md` for the full design and the root `docs/control/pulse.html` for live roadmap/todo status. Per the product spec's own instruction, implementation proceeds in vertical, independently-validated slices - Phase 4 (Basic Pulse, the product's core mechanic) is next.
