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

Architecture audit complete (this commit). No feature code has been written yet - see `docs/ARCHITECTURE_AUDIT.md`'s "First code change recommended" section and the roadmap dashboard for what Phase 1 actually is. Per the product spec's own instruction, implementation proceeds in vertical, independently-validated slices, never as one large change.
