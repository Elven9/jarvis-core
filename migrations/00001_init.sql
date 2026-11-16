-- +goose Up

-- Current state of each session: its latest upload (last write wins).
CREATE TABLE sessions (
    type              TEXT    NOT NULL,
    machine_id        TEXT    NOT NULL,
    session_id        TEXT    NOT NULL,
    blob_hash         TEXT    NOT NULL CHECK (length(blob_hash) = 64), -- sha256, lowercase hex
    size              INTEGER NOT NULL CHECK (size >= 0),
    first_uploaded_at INTEGER NOT NULL, -- unix milliseconds
    last_uploaded_at  INTEGER NOT NULL, -- unix milliseconds
    PRIMARY KEY (type, machine_id, session_id)
) STRICT, WITHOUT ROWID;

-- Every accepted upload, in the order it was recorded. A log, not versioning.
CREATE TABLE uploads (
    id          INTEGER PRIMARY KEY,
    type        TEXT    NOT NULL,
    machine_id  TEXT    NOT NULL,
    session_id  TEXT    NOT NULL,
    blob_hash   TEXT    NOT NULL CHECK (length(blob_hash) = 64),
    size        INTEGER NOT NULL CHECK (size >= 0),
    uploaded_at INTEGER NOT NULL, -- unix milliseconds
    FOREIGN KEY (type, machine_id, session_id) REFERENCES sessions (type, machine_id, session_id)
) STRICT;

CREATE INDEX uploads_by_session ON uploads (type, machine_id, session_id, id);
