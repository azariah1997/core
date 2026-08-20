CREATE TABLE IF NOT EXISTS bonds (
    id              TEXT PRIMARY KEY,
    relationship_id TEXT NOT NULL UNIQUE,
    user_a          TEXT NOT NULL,
    user_b          TEXT NOT NULL,
    status          TEXT NOT NULL,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at     TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The real one-active-bond-per-user enforcement (apps/pulse/docs/
-- ARCHITECTURE_AUDIT.md's Risk #2): a user can hold at most one row
-- here, ever, regardless of which bond or which side (user_a/user_b)
-- they're on. This PRIMARY KEY - not application logic - is what makes
-- two concurrent Accept calls race-safe: whichever transaction's INSERT
-- loses hits a real unique-constraint violation and rolls back.
CREATE TABLE IF NOT EXISTS bond_active_holders (
    user_id TEXT PRIMARY KEY,
    bond_id TEXT NOT NULL REFERENCES bonds(id)
);
