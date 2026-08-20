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

Phases 1-5 complete and live-validated: Product Foundation, Connection Experience, Partner Bond, Basic Pulse, and Push Fallback. `pulse-interactions` decides Live vs. Push once per interaction, from a real presence check against `realtime-gateway` - Live delivers PulseStart/PulseStop over a real WebSocket connection (via `platformkit/rtbus`); Push requests a real notification through Core's `notifications.Send` once the interaction completes, carrying the real final duration. Duration is always computed server-side from its own timestamps, creation is idempotent for mobile network retries, sends are rate-limited, and every completed Pulse records a durable `pulse_completed` analytics event. `apps/pulse/mobile` ships a real `HapticEngine` (Standard tier via Flutter's cross-platform `HapticFeedback`) and a working hold-to-Pulse gesture wired end to end.

Phase 5 required two real, carefully-scoped Core changes, both decided with the user's explicit sign-off since they touch shared platform capabilities rather than Pulse's own code: `realtime-gateway` gained `GET /v1/presence/{userId}` (exposing its already-existing presence tracker), and `notifications.Send` was relaxed from self-or-platform.admin to any authenticated caller, attributed via a new `SentByUserID` field - Pulse's sender-notifies-receiver need turned out to be the first real cross-user notification requirement this platform had ever had.

Live-validated end to end, both paths: a real WebSocket round trip for Live delivery, and for Push - a real presence check confirming `pulsefriend` was genuinely offline, a real notification landing in Core's database attributed to the real sender, an honest real failure ("recipient has no device with a registered push token"), and then a genuine real success once a real device/push-token was registered. See `docs/ARCHITECTURE_AUDIT.md` for the full design and the root `docs/control/pulse.html` for live roadmap/todo status, including the honestly-scoped gaps (Advanced-tier native haptics, real iOS/Android device testing) still open. Per the product spec's own instruction, implementation proceeds in vertical, independently-validated slices - Phase 6 (Pulse Back) is next.
