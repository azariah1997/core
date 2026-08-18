# Messaging

Durable conversations and messages built on top of generic platform capabilities, per the roadmap: `Conversation`, `ConversationMember`, `Message`, `Delivery`, `ReadState`, `Reaction`, `Attachment`. Realtime delivery and durable storage are deliberately separate concerns - a message is durably written first, then pushed to connected clients over the Realtime Gateway (Phase 10), never the other way around.

## Responsibilities

- Own `Conversation` (`direct`/`group`/`custom` - a closed set the roadmap itself names) and its members.
- Own `Message`, with a free-form, product-defined `Type` (TEXT/IMAGE/FILE/SYSTEM/CUSTOM are the roadmap's own examples of a type future products can extend) - never validated against a fixed enum, the same convention this repo already applies to `RelationshipType` and `GroupMember.Role`.
- Own `Attachment` (a reference row - URL, content type, size - not an upload pipeline; that's Phase 13/File-Media Platform's job), `Delivery` (per-recipient, client-acknowledged receipt), `ReadState` (per-member read position), and `Reaction` (free-form type, same convention as `Message.Type`).
- Write the message durably (plus attachments, per-recipient `Delivery` rows, and the `message.sent` outbox event) in one transaction, then - only once that's committed - best-effort push a `message.new` event to every other member over `platformkit/rtbus`, the same Redis pub/sub contract realtime-gateway's hub already listens on. A push failure never fails the send.

## Scoping decisions

- **Direct-conversation dedup**: creating a `direct` conversation between a pair that already has one returns the existing conversation instead of a duplicate thread - what every real "message this person" action expects. Enforced at the application layer (`Service.CreateConversation` checks `FindDirectBetween` before inserting), not a DB constraint, since the member pair lives in `conversation_members`, a separate table from `conversations` - a known, accepted race on truly concurrent creates, documented in `0010_messaging.sql`.
- **Membership management**: `group`/`custom` conversations let any existing member add another; `direct` conversations reject membership changes outright (the two participants are fixed at creation). Removal only supports self-removal ("leave") - unlike `groups`, the roadmap defines no role/permission concept for conversations that would justify letting one member remove another, so that's deferred.
- **Delivery acknowledgement travels over REST, not WebSocket**: Phase 10's WebSocket protocol is generic pub/sub transport, not domain-specific acks, so `POST .../messages/{id}/delivered` is a plain authenticated REST call rather than a new WS message type.
- **Reactions and delivery acks are idempotent upserts**: reacting twice with the same type, or acknowledging delivery twice, is a no-op returning the existing row rather than a conflict - reactions are a toggleable client affordance, and delivery acks are a common double-fire client race, neither is a genuine resource conflict.

## Layout

- `domain.go` - types, validation, `Repository` interface.
- `service.go` - `Service`, all membership/permission checks, and the realtime push after a successful send. Depends on a narrow `Realtime` interface (`ToUser(ctx, userID, payload) error`), satisfied directly by `platformkit/rtbus.Publisher` in production and a fake in tests - the same consumer-defined-interface pattern used elsewhere in this repo (`users.AccessChecker`, `users.IdentityLinker`) to avoid a domain module depending on another concrete package it doesn't need in full.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `postgres/` - the production `Repository`. Builds on the pre-existing `conversations`/`conversation_members`/`messages` tables from the original scaffold (kept their column names - `message_type`, `payload` - the same way `relationships` kept `user_a`/`user_b`) plus the new tables `0010_messaging.sql` adds for attachments, deliveries, read state and reactions.
- `memory/` - in-memory `Repository` for tests.

## Storage

`conversations`, `conversation_members`, `messages` (`data/migrations/0001_core.sql`, extended by `0010_messaging.sql` with `conversations.metadata`/`updated_at`), plus `message_attachments`, `message_deliveries`, `conversation_read_states`, `message_reactions` (`0010_messaging.sql`).

## Realtime bridge

`packages/go/platformkit/rtbus` is the shared wire contract (envelope shape, Redis channel-prefix constants) both this package and `realtime-gateway/internal/hub` depend on - extracted during this phase so two services publishing/consuming the same pub/sub messages can't drift into incompatible JSON shapes. `Service.SendMessage` publishes a `{"type":"message.new","conversationId","message":{...}}` payload to every other member via `rtbus.Publisher.ToUser`; a client connected to realtime-gateway receives it verbatim on its WebSocket, no action (subscribe, etc.) required - core-api and realtime-gateway are different processes that have never called each other directly, and never will; Redis pub/sub is the only thing connecting them.
