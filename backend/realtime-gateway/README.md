# Realtime Gateway

A dedicated WebSocket service for presence and low-latency, non-durable fan-out (channel broadcast, direct-to-user, direct-to-device). Not a message store - if nobody's connected when something is published, it's simply not delivered. Durable messaging (offline delivery, history, read receipts) is Phase 11's job, built on top of this transport, not inside it.

## Why a separate service

Deliberately split from `core-api` rather than bolted onto it: WebSocket connections are long-lived and stateful in a way REST handlers aren't, the Helm chart scales it independently (`realtime.replicas: 2`), and it only ever *reads* from Postgres (to resolve an authenticated identity to a platform user ID) - it never writes there. Everything routable across replicas goes through Redis/Valkey, never process memory alone.

## Protocol

`GET /ws?access_token=<JWT>&deviceId=<string>` upgrades to a raw RFC6455 WebSocket (hand-rolled framing in `internal/ws`, no third-party WS library, consistent with this repo's existing preference for owning its own protocol code over pulling in a dependency for something this small). The token travels as a query parameter because browsers can't set an `Authorization` header on the handshake request - the standard fallback for this transport.

On accept, the server sends `{"type":"connected","connectionId","userId","deviceId"}`. From there, client messages are JSON text frames:

- `{"type":"subscribe","channel":"..."}` -> `{"type":"subscribed","channel":"..."}`
- `{"type":"unsubscribe","channel":"..."}` -> `{"type":"unsubscribed","channel":"..."}`
- `{"type":"publish","channel":"...","data":{...}}` -> broadcast to every subscriber of that channel, on every replica, as `{"type":"message","channel","fromUserId","data"}`. Requires having subscribed first (`{"type":"error"}` otherwise). The publisher, if also subscribed, receives its own broadcast like any other subscriber - there's no self-exclusion.
- `{"type":"direct","targetUserId":"...","data":{...}}` -> delivered to every device `targetUserId` is connected on (any replica), as `{"type":"direct","fromUserId","data"}`.

The server pings idle connections every 25s and expects a pong or any client activity within 70s, or the connection is dropped.

### Server-initiated pushes from other services

Not every message a client receives originates from a `publish`/`direct` client action. Any service holding a `platformkit/rtbus.Publisher` on the same Redis can push directly - `core-api`'s messaging module (Phase 11) does exactly this: `Service.SendMessage` publishes a `{"type":"message.new","conversationId","message":{...}}` payload to every other conversation member via `rtbus.Publisher.ToUser`, and a connected client receives it verbatim with no `subscribe` or other action required. `core-api` and `realtime-gateway` never call each other directly - Redis pub/sub (`packages/go/platformkit/rtbus`) is the only thing connecting them, which is also why the wire envelope lives in a shared platformkit package rather than being private to this service's `hub` package.

## Presence query (`GET /v1/presence/{userId}`)

A plain, authenticated (`Authorization: Bearer <token>`, standard header - unlike `/ws`, this isn't a WebSocket handshake so there's no query-param workaround needed) REST endpoint answering `{"userId","online"}` - is this user connected on any device, right now. Backed directly by `internal/presence.Tracker.IsOnline`, which already existed and was already used internally by `internal/ws`; this just exposes it. Added for Pulse's Phase 5 (a product backend deciding whether to attempt live delivery or fall back to a push notification) - any authenticated caller may query any user's presence, the same coarse trust level realtime delivery itself already has (any authenticated backend can already push to any user via `rtbus` with no relationship check). A coarse online/offline signal only, never exact activity or a last-seen timestamp.

## Packages

- `internal/ws` - the connection handler: HTTP hijack + RFC6455 handshake (`handler.go`), frame read/write (`frame.go`), the client/server message shapes (`messages.go`), and per-request auth context (`auth.go`). All writes to a connection's socket - data frames, ping, pong replies - funnel through one `writeLoop` goroutine; `net.Conn` only guarantees safety for one concurrent reader and one concurrent writer, not two writers, so nothing else is allowed to write directly.
- `internal/hub` - process-local connection registry (`conns`, channel subscriptions, `userID -> deviceID -> conn`) plus Redis pub/sub fan-out so a publish on one replica reaches subscribers connected to any replica. `Register` evicts any existing local connection for the same `(userID, deviceID)` - the same-replica reconnect case - by closing the old connection's `Send` channel, which its `writeLoop` treats as a signal to also close the socket, promptly unblocking its `readLoop` rather than leaving it to linger until the 70s read timeout.
- `internal/presence` - `userID`/`deviceID` -> connection ID in Redis with a TTL (`presence:<userID>:<deviceID>`), refreshed on every client message. An unclean disconnect (crash, network partition) simply ages out via TTL rather than needing an explicit cleanup path.
- `internal/identity` - a narrow, read-only `UserIDForSubject(provider, subject) -> userID` query against the shared `identities` table. Justified despite `core-api` owning the identity domain because this service shares the platform's single Postgres by architectural design and only ever needs this one lookup - duplicating the whole identity domain here would be worse than one narrow read.

## Known limitation

Eviction on reconnect only works within a single replica. If a user's existing connection lives on a different replica than the one their reconnect lands on (routed there by the load balancer), the old connection isn't torn down - it just goes quiet until its own ping/pong or read timeout notices. A cross-replica "kick" broadcast (publish an evict message the owning replica listens for) would close this gap; not built yet because nothing in this phase's scope required more than one realtime-gateway replica running locally to validate against.

## Auth

Reuses `packages/go/platformkit/jwtverify` (new this phase) rather than `core-api/internal/identity`'s Keycloak provider - this service only ever needs to verify a bearer token's signature/claims, never to mint tokens or talk to Keycloak's admin API, so it gets the narrower dependency.
