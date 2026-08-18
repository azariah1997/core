BEGIN;

-- The scaffold's conversations table had no metadata/updated_at - added
-- here rather than at table-creation time, same additive-migration
-- preference used for users.avatar_ref in 0004.
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

-- Direct-conversation dedup (at most one thread per app+member-pair) is
-- enforced at the application layer (Service.CreateConversation calls
-- FindDirectBetween before inserting) rather than a DB constraint: the
-- member pair lives in conversation_members, a separate table, so
-- expressing "unique per pair" as a single-table index here isn't
-- possible without duplicating membership data onto conversations. A
-- known, accepted limitation: two truly concurrent creates for the same
-- pair could race into two rows, same class of tradeoff already
-- documented for realtime-gateway's cross-replica eviction gap.
CREATE TABLE IF NOT EXISTS message_attachments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  url text NOT NULL,
  content_type text NOT NULL DEFAULT '',
  size_bytes bigint NOT NULL DEFAULT 0,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS ix_message_attachments_message ON message_attachments(message_id);

-- One row per (message, intended recipient), created for every other
-- conversation member at send time; delivered_at is set by an explicit
-- client acknowledgement (Phase 10's WebSocket protocol is generic
-- transport, not domain-specific acks, so this travels over REST).
CREATE TABLE IF NOT EXISTS message_deliveries (
  message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  delivered_at timestamptz,
  PRIMARY KEY(message_id, user_id)
);

-- Its own record rather than a field on conversation_members - membership
-- changes rarely, read position potentially on every message view.
CREATE TABLE IF NOT EXISTS conversation_read_states (
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  last_read_message_id uuid REFERENCES messages(id),
  last_read_at timestamptz,
  PRIMARY KEY(conversation_id, user_id)
);

-- reaction_type is free-form (an emoji or a product-defined name), same
-- convention as relationship_type and group_members.role - never
-- validated against a platform enum.
CREATE TABLE IF NOT EXISTS message_reactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reaction_type text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_message_reactions_unique ON message_reactions(message_id, user_id, reaction_type);

COMMIT;
