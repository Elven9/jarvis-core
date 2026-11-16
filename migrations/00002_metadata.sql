-- +goose Up

-- Parsed metadata of each session's latest accepted upload, as the session-parser
-- library returned it (Metadata). Replaced whole on every accepted upload.
-- Each row belongs to exactly one session record.
CREATE TABLE session_metadata (
    type              TEXT    NOT NULL,
    machine_id        TEXT    NOT NULL,
    session_id        TEXT    NOT NULL,
    parser_version    TEXT    NOT NULL, -- session-parser module version
    header_session_id TEXT    NOT NULL, -- the session header's id
    cwd               TEXT    NOT NULL,
    started_at        INTEGER NOT NULL, -- unix milliseconds
    format_version    INTEGER NOT NULL,
    parent_session    TEXT,             -- NULL when not forked
    agent_version     TEXT,             -- NULL without a pi-agent.session-version entry
    name              TEXT,             -- NULL when unnamed
    tags_source       TEXT,             -- NULL without a pi-agent.tags entry
    tags_at           INTEGER,          -- unix milliseconds; NULL with tags_source
    CHECK ((tags_source IS NULL) = (tags_at IS NULL)),
    PRIMARY KEY (type, machine_id, session_id),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES sessions (type, machine_id, session_id)
) STRICT, WITHOUT ROWID;

-- The session's tags in their order; no rows for an empty (cleared) list.
CREATE TABLE session_tags (
    type       TEXT    NOT NULL,
    machine_id TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    tag        TEXT    NOT NULL,
    PRIMARY KEY (type, machine_id, session_id, position),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metadata ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

-- Model changes on the current branch, in order.
CREATE TABLE session_models (
    type       TEXT    NOT NULL,
    machine_id TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    provider   TEXT    NOT NULL,
    model_id   TEXT    NOT NULL,
    at         INTEGER NOT NULL, -- unix milliseconds
    PRIMARY KEY (type, machine_id, session_id, position),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metadata ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

-- pi-agent.error entries on the current branch, in order.
CREATE TABLE session_errors (
    type       TEXT    NOT NULL,
    machine_id TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    message    TEXT    NOT NULL,
    at         INTEGER NOT NULL, -- unix milliseconds
    PRIMARY KEY (type, machine_id, session_id, position),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metadata ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

-- Metrics totals of the current branch (the newest pi-agent.metrics entry); no row
-- for a session without metrics. Raw samples are not stored.
CREATE TABLE session_metrics (
    type                 TEXT    NOT NULL,
    machine_id           TEXT    NOT NULL,
    session_id           TEXT    NOT NULL,
    tokens_input         INTEGER NOT NULL,
    tokens_output        INTEGER NOT NULL,
    tokens_cache_read    INTEGER NOT NULL,
    tokens_cache_write   INTEGER NOT NULL,
    cost_usd             REAL    NOT NULL,
    turns                INTEGER NOT NULL,
    model_calls          INTEGER NOT NULL,
    ttft_ms_p50          REAL,    -- NULL without a model request
    latency_ms_p50       REAL,
    latency_ms_max       REAL,
    compactions          INTEGER NOT NULL,
    context_peak_percent REAL,    -- NULL when unknown
    entries              INTEGER NOT NULL,
    entry_id             TEXT    NOT NULL,
    at                   INTEGER NOT NULL, -- unix milliseconds
    PRIMARY KEY (type, machine_id, session_id),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metadata ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

-- Per-tool totals of the session's metrics.
CREATE TABLE session_tool_metrics (
    type       TEXT    NOT NULL,
    machine_id TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    tool       TEXT    NOT NULL,
    calls      INTEGER NOT NULL,
    errors     INTEGER NOT NULL,
    ms_p50     REAL    NOT NULL,
    ms_max     REAL    NOT NULL,
    PRIMARY KEY (type, machine_id, session_id, tool),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metrics ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

-- Requests that read less from the prompt cache than expected, in order.
CREATE TABLE session_cache_misses (
    type       TEXT    NOT NULL,
    machine_id TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    turn       INTEGER NOT NULL,
    expected   INTEGER NOT NULL,
    cache_read INTEGER NOT NULL,
    PRIMARY KEY (type, machine_id, session_id, position),
    FOREIGN KEY (type, machine_id, session_id) REFERENCES session_metrics ON DELETE CASCADE
) STRICT, WITHOUT ROWID;
