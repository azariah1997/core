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

Phases 1-4 complete and live-validated: Product Foundation, Connection Experience, Partner Bond, and **Basic Pulse - the product's actual core mechanic**. `pulse-interactions` delivers real PulseStart/PulseStop over a real WebSocket connection through `realtime-gateway` (via `platformkit/rtbus`, the same shared contract Core's own messaging module uses), computes duration server-side from its own timestamps (never trusting the client), supports idempotent creation for mobile network retries, is rate-limited, and records a durable `pulse_completed` event through Core's real analytics ingest. `apps/pulse/mobile` ships a real `HapticEngine` (Standard tier via Flutter's cross-platform `HapticFeedback`) and a working hold-to-Pulse gesture wired end to end.

Live-validated with two real Keycloak users and a real WebSocket connection: full create → start → live push → stop → live push round trip, server-computed duration matching the real hold time, the analytics event landing in Core's own database, rate limiting kicking in at exactly the configured boundary, and both "not connected" and "blocked" rejections. See `docs/ARCHITECTURE_AUDIT.md` for the full design and the root `docs/control/pulse.html` for live roadmap/todo status, including the honestly-scoped gaps (offline push fallback, Advanced-tier native haptics, real device testing) still open. Per the product spec's own instruction, implementation proceeds in vertical, independently-validated slices - Phase 5 (Push Fallback) is next.
