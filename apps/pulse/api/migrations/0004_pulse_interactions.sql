CREATE TABLE IF NOT EXISTS interactions (
    id                 TEXT PRIMARY KEY,
    type               TEXT NOT NULL,
    sender_id          TEXT NOT NULL,
    receiver_id        TEXT NOT NULL,
    started_at         TIMESTAMPTZ,
    ended_at           TIMESTAMPTZ,
    duration_ms        INTEGER,
    delivery_mode      TEXT NOT NULL,
    status             TEXT NOT NULL,
    -- Idempotency (product spec §76): a retried create with the same
    -- (sender_id, client_request_id) must return the original row, not
    -- create a duplicate Pulse. NULL client_request_id never collides
    -- with itself under a UNIQUE index (standard Postgres NULL
    -- semantics), so callers who omit it are simply not deduplicated.
    client_request_id TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sender_id, client_request_id)
);

CREATE INDEX IF NOT EXISTS ix_interactions_sender ON interactions (sender_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_interactions_receiver ON interactions (receiver_id, created_at DESC);
