-- The Friend/Close-Friend overlay only - the connection lifecycle
-- itself (request/accept/decline/remove) lives entirely in Core's
-- relationships table, never duplicated here. relationship_id
-- references Core's Relationship.ID by value only (cross-database,
-- so no real foreign key).
CREATE TABLE IF NOT EXISTS pulse_connection_classifications (
    relationship_id TEXT NOT NULL,
    owner_user_id   TEXT NOT NULL,
    classification  TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (relationship_id, owner_user_id)
);
