# Parsing: job queue, re-parse and session types (later)

Goal: move parsing off the upload request and support more than one session type.
v0.2 parses inline on upload with the pi-agent session-parser library ([v0.2/metadata-storage-todo.md](v0.2/metadata-storage-todo.md)).
The format research and the pi parser (old P4, P5) live in pi-agent: `go/pkg/session-parser`.

| # | Task | Done when |
|---|------|-----------|
| P1 | `JobQueue` interface plus SQLite implementation: enqueue, lease, retry/backoff, idempotency (keyed by blob hash, so a re-upload re-parses and a stale job is dropped) | Passes the queue contract tests, including crash/lease-expiry cases |
| P2 | Worker runtime: a goroutine pool inside the server process, graceful shutdown | A queued job runs and finishes |
| P3 | Hook upload into the queue: `PUT` enqueues; the session record gets a parse status (`pending`/`parsed`/`failed`); manual re-parse, including after a library upgrade | Upload, then the job runs, then the status becomes `parsed` |
| P6 | **Session types**: how a non-`pi` type gets set (upload has no type field), a second type (gemini?) to test a `SessionParser` abstraction | The abstraction holds without changes to pi code |
