-- Experience Controls (product spec §33, §34, §72, Phase 13): the only
-- fields genuinely Pulse-owned - notification detail level and haptic
-- intensity. Quiet Hours and Mute deliberately have no table here at
-- all: Quiet Hours values live in Core's notifications.QuietHours
-- (scoped by Pulse's AppID) and Mute is Core's platform-wide
-- trustsafety.Mute - both real, already-live Core capabilities this
-- module is a thin settings surface over, never a duplicate store.
CREATE TABLE IF NOT EXISTS pulse_preferences (
    user_id              TEXT PRIMARY KEY,
    notification_detail  TEXT NOT NULL DEFAULT 'detailed',
    haptic_intensity     REAL NOT NULL DEFAULT 1.0,
    updated_at           TIMESTAMPTZ NOT NULL
);
