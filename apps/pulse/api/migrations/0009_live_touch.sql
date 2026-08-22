-- Live Touch (product spec §21, Phase 10): session lifecycle only
-- (invite/accept/decline/end) - the touch-start/touch-stop events
-- themselves are never persisted, exchanged directly between
-- participants over the session's own realtime-gateway channel.
CREATE TABLE IF NOT EXISTS live_touch_sessions (
    id            TEXT PRIMARY KEY,
    initiator_id  TEXT NOT NULL,
    receiver_id   TEXT NOT NULL,
    status        TEXT NOT NULL,
    end_reason    TEXT,
    delivery_mode TEXT NOT NULL,
    invited_at    TIMESTAMPTZ NOT NULL,
    accepted_at   TIMESTAMPTZ,
    ended_at      TIMESTAMPTZ,
    duration_ms   INTEGER,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_live_touch_sessions_initiator ON live_touch_sessions (initiator_id, invited_at DESC);
CREATE INDEX IF NOT EXISTS ix_live_touch_sessions_receiver ON live_touch_sessions (receiver_id, invited_at DESC);
