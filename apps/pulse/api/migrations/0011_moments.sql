-- Moments (product spec §30-31, Phase 12): a private saved-highlights
-- timeline - "avoid turning this into chat history." A Moment is a
-- personal bookmark: each participant may independently save the same
-- underlying interaction, so (saved_by_user_id, interaction_id) is
-- unique, never a single shared row. Stores exactly what spec §31
-- names (timestamp, participants, interaction type, duration/pattern)
-- - no message content, ever.
CREATE TABLE IF NOT EXISTS moments (
    id                TEXT PRIMARY KEY,
    interaction_id    TEXT NOT NULL,
    saved_by_user_id  TEXT NOT NULL,
    other_user_id     TEXT NOT NULL,
    interaction_type  TEXT NOT NULL,
    duration_ms       INTEGER,
    pattern           TEXT,
    occurred_at       TIMESTAMPTZ NOT NULL,
    saved_at          TIMESTAMPTZ NOT NULL,
    UNIQUE (saved_by_user_id, interaction_id)
);

CREATE INDEX IF NOT EXISTS ix_moments_saved_by ON moments (saved_by_user_id, saved_at DESC);
