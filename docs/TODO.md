# Server TODO index (before v1)

Source: [proposal.md](proposal.md) (the original proposal)

Tracks the whole server up to v1. Each version's own docs live in its folder:

- **v0.1** ([v0.1/](v0.1/)): session storage only: upload, store and read back raw sessions.
- **After v0.1**: parsing and the metadata store.

Approach: design the core pieces first, then build thin end-to-end slices, each tested against real uploads.
The task lists are settled. Tasks get no detail docs: the code is the truth, and the tests are the list of behaviors.

## Todo files

| File | Scope | Version |
|------|-------|---------|
| [v0.1/tech-stack-decision.md](v0.1/tech-stack-decision.md) | Tech stack decisions (REST, Fiber, SQLite, goose, logging, config, container) | v0.1, decided |
| [v0.1/session-store-todo.md](v0.1/session-store-todo.md) | Upload and store raw sessions | v0.1, first |
| [parsing-todo.md](parsing-todo.md) | Job queue, parsing raw sessions, session types | After v0.1 |
| [metadata-storage-todo.md](metadata-storage-todo.md) | Where parsed data and time-series metrics live | After v0.1 |
| [result-reference-todo.md](result-reference-todo.md) | Tool-call refs, reports, references (a separate feature) | Not planned yet |

Order: v0.1 is the session store. After v0.1: metadata-storage design, then parsing. Parsing writes into the metadata store, so the store's design has to come first.

## Decisions so far

- **Upload contract**: already fixed by the pi-agent `session-upload` extension (`~/Project/agents-infra/pi-agent/src/session-upload/upload.ts`):
  - `PUT <endpoint>/v1/sessions/<machineId>/<sessionId>`, body = the raw JSONL session file, `content-type: application/x-ndjson`.
  - `machineId` matches `^[A-Za-z0-9._-]+$`; both path segments are URL-encoded.
  - The server replaces whatever it has. Any 2xx means success; on an error the client shows the first 200 chars of the body.
  - Client: 15 s timeout, no retry. Extra headers from the client config. The server ignores them because there is no auth.
  - The request carries no session type and no tool-output files.
  - Change pending: the extension still calls the unprefixed `/sessions/...`; it moves to `/v1` before task 1.4 ([v0.1/tech-stack-decision.md](v0.1/tech-stack-decision.md#api-style-rest)).
- **No auth in v1**: personal system; auth lives in the infrastructure (e.g. Cloudflare Access).
- **No versioning in v1**: a re-upload overwrites. The upload history table records every upload (hash, size, time), but it is a log, not versioning: old versions cannot be read back through the API.
- **No retention in v1**: neither the policy nor the code.
- **Store raw, parse through a queue**: upload stores the raw bytes (v0.1); a background job parses them later ([parsing-todo.md](parsing-todo.md)).
- **Storage**: local filesystem (blobs) plus SQLite to start, behind clean Go interfaces so we can swap in stronger stores later.
- **Global session ID**: `(type, machineId, sessionId)`, case-sensitive; every upload is type `pi` for now. Details in [v0.1/session-store-todo.md](v0.1/session-store-todo.md#decisions). Setting other types belongs to P6.
- **Tool-output files** (full output behind truncated tool results): out of scope.

## Later (not assigned to a todo file yet)

- Session search (text search, Chinese, inactive-branch marking)
- Dashboard (no SPA framework)
- Retention policy and GC (including blobs orphaned by overwrites; v0.1 deletes nothing)
- List sessions API
- Session versioning
- Migration to stronger stores (S3/Postgres) through the storage interfaces
