-- Custom Signals (product spec §19-20, Phase 11): a private touch
-- pattern library, bound to exactly one specific connection - "the same
-- pattern means something different to different pairs, so it only
-- ever means one thing: the relationship it was created for." The
-- platform never interprets segments' meaning, only stores and relays
-- them (spec §20).
CREATE TABLE IF NOT EXISTS custom_signals (
    id              TEXT PRIMARY KEY,
    owner_user_id   TEXT NOT NULL,
    target_user_id  TEXT NOT NULL,
    label           TEXT NOT NULL DEFAULT '',
    -- segments is a JSON array of {"type": "tap"|"hold"|"pause", "durationMs": int} -
    -- bounds (max 20 segments, 20-3000ms each, 10000ms total) are
    -- enforced server-side at creation time (Architecture Audit Risk #4),
    -- never merely a client-side UI limit.
    segments        JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_custom_signals_owner ON custom_signals (owner_user_id, created_at DESC);
