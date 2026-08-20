-- Today's Mood (product spec §22-27, Phase 8): a singleton row per
-- user, replaced wholesale by the next Set - never a history table
-- (spec §26: "historical Mood storage is a separate product decision").
-- allowed_viewer_ids is the audience resolved once, at Set time, from
-- the owner's own real connections/bond state (see mood.Mood's own doc
-- comment) - never re-derived per viewer.
CREATE TABLE IF NOT EXISTS moods (
    user_id            TEXT PRIMARY KEY,
    emoji              TEXT NOT NULL,
    audience           TEXT NOT NULL,
    allowed_viewer_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL
);

-- Get filters WHERE expires_at > now() on every read (spec §26: an
-- expired Mood is functionally gone, not merely marked "expired" and
-- still returned) - this index makes that filter cheap without a
-- background sweep job.
CREATE INDEX IF NOT EXISTS ix_moods_expires_at ON moods (expires_at);
