-- Pulse's own database - never Core's. user_id references a Core
-- User.ID by value only (no cross-database foreign key is possible or
-- desired; Pulse resolves the caller through Core's API, see
-- internal/pulseauth).
CREATE TABLE IF NOT EXISTS pulse_profiles (
    user_id       TEXT PRIMARY KEY,
    handle        TEXT NOT NULL UNIQUE,
    visual_prefs  JSONB NOT NULL DEFAULT '{}',
    pulse_prefs   JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
