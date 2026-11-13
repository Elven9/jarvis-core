# Parsing: job queue and raw session parsing (after v0.1)

Goal: every upload queues a parse job; a worker parses the raw session and writes the results into the metadata store.
Depends on [v0.1/session-store-todo.md](v0.1/session-store-todo.md) (raw sessions exist) and [metadata-storage-todo.md](metadata-storage-todo.md) (where the results go).

| # | Task | Done when |
|---|------|-----------|
| P1 | `JobQueue` interface plus SQLite implementation: enqueue, lease, retry/backoff, idempotency (keyed by blob hash, so a re-upload re-parses and a stale job is dropped) | Passes the queue contract tests, including crash/lease-expiry cases |
| P2 | Worker runtime: a goroutine pool inside the server process, graceful shutdown | A queued job runs and finishes |
| P3 | Hook upload into the queue: `PUT` enqueues; the session record gets a parse status (`pending`/`parsed`/`failed`); manual re-parse | Upload, then the job runs, then the status becomes `parsed` |
| P4 | **Research**: pi session JSONL format: entries, branches, compaction, the metrics totals the metrics extension appends (use `pi-agent/src/tool-ref`, `src/metrics`, `testdata/*.jsonl`) | Format notes plus golden test files |
| P5 | `SessionParser` interface keyed by session type, plus the pi parser | Golden tests pass on real sessions |
| P6 | **Session types**: how a non-`pi` type gets set (upload has no type field), a second type (gemini?) to test the `SessionParser` abstraction | The abstraction holds without changes to pi code |
