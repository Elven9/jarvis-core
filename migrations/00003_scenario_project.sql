-- +goose Up

-- The scenario of the last pi-agent.scenario entry in the file; NULL without one, or when
-- that entry records none. Metadata stored before this migration has NULL until the
-- session's next accepted upload.
ALTER TABLE session_metadata ADD COLUMN scenario TEXT;

-- The project of the last pi-agent.project entry in the file; no row without one, or when
-- that entry records none (and for metadata stored before this migration, until the
-- session's next accepted upload). The source fields are stored as the parser returned
-- them: dir for a local project; provider, repo, path, tag and commit for a remote one.
CREATE TABLE session_projects (
    type            TEXT    NOT NULL,
    machine_id      TEXT    NOT NULL,
    session_id      TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    source_kind     TEXT    NOT NULL CHECK (source_kind IN ('local', 'remote')),
    source_dir      TEXT    NOT NULL,
    source_provider TEXT    NOT NULL,
    source_repo     TEXT    NOT NULL,
    source_path     TEXT    NOT NULL,
    source_tag      TEXT    NOT NULL,
    source_commit   TEXT    NOT NULL,
    at              INTEGER NOT NULL, -- unix milliseconds
    PRIMARY KEY (type, machine_id, session_id),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metadata ON DELETE CASCADE
) STRICT, WITHOUT ROWID;
