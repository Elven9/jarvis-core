-- +goose Up

-- The time of the newest user or assistant message anywhere in the file, in unix
-- milliseconds (the parser's started_at when there is none). Metadata stored before this
-- migration has NULL until the session's next accepted upload; it reads as started_at.
ALTER TABLE session_metadata ADD COLUMN last_activity_at INTEGER;
