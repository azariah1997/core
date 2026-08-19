# Communication module

Owns the **communication** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented across two services. `backend/realtime-gateway` is a standalone service owning WebSocket connections, channel subscriptions, and presence (TTL-based, Valkey-backed) - hand-rolled RFC6455, with Redis-fanned pub/sub so delivery works across replicas. `backend/core-api/internal/messaging` owns durable storage - `Conversation`/`Message`/`Delivery`/`ReadState`/`Reaction`/`Attachment` - and publishes the real-time fan-out through a shared `rtbus` package rather than either service reaching into the other's internals. Exposed at `/v1/conversations*` (core-api) and `ws://.../ws` (realtime-gateway). See each package's README for detail.
