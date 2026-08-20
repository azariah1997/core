-- selected_circles audience (product spec §25, Phase 9): CircleID is
-- kept alongside the already-resolved allowed_viewer_ids purely for the
-- owner's own management view, the same way custom_users' chosen list
-- is echoed back - NULL for every other audience type.
ALTER TABLE moods ADD COLUMN IF NOT EXISTS circle_id TEXT;
