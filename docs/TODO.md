# Server TODO index (before v1)

Source: [proposal.md](proposal.md) (the original proposal)

Tracks the whole server up to v1. Each version's own docs live in its folder:

- **v0.1** ([v0.1/](v0.1/)): session storage only: upload, store and read back raw sessions.
- **v0.2** ([v0.2/](v0.2/)): parse on upload with the pi-agent session-parser library; metadata store with per-session metrics totals; meta and list routes.
- **v0.3** ([v0.3/](v0.3/)): session-parser `go/v0.2.0`; parsed session route (parsed on every request), raw download under `/raw`; scenario and project stored.
- **Later**: job queue, session types, result references.

Approach: design the core pieces first, then build thin end-to-end slices, each tested against real uploads.
The task lists are settled. Tasks get no detail docs: the code is the truth, and the tests are the list of behaviors.

## Todo files

| File | Scope | Version |
|------|-------|---------|
| [v0.1/tech-stack-decision.md](v0.1/tech-stack-decision.md) | Tech stack decisions (REST, Fiber, SQLite, goose, logging, config, container) | v0.1, decided |
| [v0.1/session-store-todo.md](v0.1/session-store-todo.md) | Upload and store raw sessions | v0.1, first |
| [v0.2/metadata-storage-todo.md](v0.2/metadata-storage-todo.md) | Parse on upload, metadata store (with metrics totals), meta and list routes | v0.2 |
| [v0.3/parsed-session-todo.md](v0.3/parsed-session-todo.md) | Parsed session route, raw route, scenario and project in the metadata store | v0.3 |
| [parsing-todo.md](parsing-todo.md) | Job queue, re-parse, session types | Later |
| [result-reference-todo.md](result-reference-todo.md) | Tool-call refs, reports, references (a separate feature) | Not planned yet |

Order: v0.1 is the session store. v0.2 parses inline on upload (no queue) into the metadata store. The queue and re-parse come later.

## Decisions so far

- **Upload contract**: already fixed by the pi-agent `session-upload` extension (`~/Project/agents-infra/pi-agent/src/session-upload/upload.ts`):
  - `PUT <endpoint>/v1/sessions/<machineId>/<sessionId>`, body = the raw JSONL session file, `content-type: application/x-ndjson`.
  - `machineId` matches `^[A-Za-z0-9._-]+$`; both path segments are URL-encoded.
  - The server replaces whatever it has. Any 2xx means success; on an error the client shows the first 200 chars of the body.
  - Client: 15 s timeout, no retry. Extra headers from the client config. The server ignores them because there is no auth.
  - The request carries no session type and no tool-output files.
- **No auth in v1**: personal system; auth lives in the infrastructure (e.g. Cloudflare Access).
- **No versioning in v1**: a re-upload overwrites. The upload history table records every upload (hash, size, time), but it is a log, not versioning: old versions cannot be read back through the API.
- **No retention in v1**: neither the policy nor the code.
- **Store raw, parse on upload**: upload stores the raw bytes (v0.1). v0.2 parses inline and rejects sessions it cannot parse ([v0.2/metadata-storage-todo.md](v0.2/metadata-storage-todo.md)); a background queue may come later ([parsing-todo.md](parsing-todo.md)).
- **Session read routes**: `GET /v1/sessions/{machineId}/{sessionId}` parses the current upload on each request with every group of the session parser (metadata, metrics with samples, tool calls with codemode scripts, subagents, skills, project instructions) and answers JSON; nothing of it is stored. A stored file the parser rejects answers a `422` problem. `.../raw` downloads the raw file (`application/x-ndjson`); `.../meta` answers the stored metadata. Details in [v0.3/parsed-session-todo.md](v0.3/parsed-session-todo.md).
- **Parser lives in pi-agent**: the Go library `github.com/Elven9/pi-agent/go` (released as `go/vX.Y.Z`), next to the extensions that write the format. jarvis-core stores only what it returns.
- **Storage**: local filesystem (blobs) plus SQLite to start, behind clean Go interfaces so we can swap in stronger stores later.
- **Global session ID**: `(type, machineId, sessionId)`, case-sensitive; every upload is type `pi` for now. Details in [v0.1/session-store-todo.md](v0.1/session-store-todo.md#decisions). Setting other types belongs to P6.
- **Tool-output files** (full output behind truncated tool results): out of scope.

## Later (not assigned to a todo file yet)

- Session search (text search, Chinese, inactive-branch marking)
- Dashboard (no SPA framework)
- Retention policy and GC (including blobs orphaned by overwrites; v0.1 deletes nothing)
- List filters (v0.2 has a plain paginated list)
- `MetricsStore` with time series, and the metrics query route (v0.2 keeps per-session totals in the `MetadataStore`)
- Session relationships (forks, subagent child sessions)
- Session versioning
- Migration to stronger stores (S3/Postgres) through the storage interfaces
