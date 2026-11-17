# Metadata store (v0.2)

Goal: an upload is parsed on the spot; its metadata (metrics totals included) is stored and can be read back per session and listed.
Replaces the old after-v0.1 `metadata-storage-todo.md`. Builds on [../v0.1/session-store-todo.md](../v0.1/session-store-todo.md).

## Decisions

### Parsing

- Parser: the pi-agent Go library `github.com/Elven9/pi-agent/go/pkg/session-parser`, imported in-process and pinned in `go.mod` (`go/v0.1.0` to start).
- The server stores only what the library returns. A missing field is added to the library first (new `go/vX.Y.Z` release), never computed in jarvis-core.
- Only the active branch counts; the library decides this (tags, name and agent version are per file, as the library defines them).
- v0.2 uses `Metadata()` and `Metrics()` only. `ToolCalls()` and `Subagents()` are not stored.
- The parser version (the module version from `debug.ReadBuildInfo`) is stored with the parsed data.
- Parsing runs inline in `PUT`. No job queue.
- Only pi session format 3 (what the library reads). Older formats are rejected; no plan to support them.

### Upload rejection (behavior change from v0.1)

- v0.1 stored any non-empty text body. v0.2 accepts only a session the library can parse.
- The upload is rejected when `Parse`, `Metadata()` or `Metrics()` fails (any group), or when the header ID differs from the URL `sessionId`.
- Rejection: `422`, short plain-text body with the parse error in the first 200 chars (what the client shows), and an error log.
- Accepted consequence: a pi-agent release with a new payload version (metrics, tags, ...) breaks uploads until jarvis-core updates the library and is redeployed. The client keeps its file, so it can re-upload.
- Unchanged from v0.1: content-type check, empty body rejected, `200` with an empty body on success.
- Upload tests switch to valid session files (golden files), with session IDs matching their headers.

### Write sequence (no transactions)

1. Store the blob.
2. Parse the stored blob.
3. Check the header ID against the URL.
4. Upsert the session record and append the upload history row.
5. Replace the session's metadata, metrics included (`null` when the new upload has none).

- A rejected upload (steps 2–3) changes no record; its blob stays, unreferenced (nothing deletes blobs yet).
- No transaction across the steps. A failed step writes an error log, fails the request (`500`) and rolls nothing back, so a session can be left mixed (e.g. new record, old metadata) until its next successful upload. Finer handling later, if ever needed.
- A successful upload leaves no stale rows from the previous one.

### Storage

- One `MetadataStore`: a Go interface with a shared contract test suite. SQLite implementation in the same database file and goose migration sequence; swappable later.
- Metrics are per-session totals only (the library's `Metrics`); no time series and no raw samples. They are a separate, nullable part of a session's metadata (`null` for a session without `pi-agent.metrics` entries), so a later `MetricsStore` for time series can sit next to it without moving the totals.
- Times are unix milliseconds (`INTEGER`), like the v0.1 tables. No timezone handling.

### Read API

- `GET /v1/sessions/{machineId}/{sessionId}/meta`: the parsed metadata, the metrics (`null` when none) and the parser version, as JSON. Errors use RFC 9457 `application/problem+json`, like the raw read route (now `GET /v1/sessions/{machineId}/{sessionId}/raw`, see [../TODO.md](../TODO.md#decisions-so-far)).
- `GET /v1/sessions`: list sessions, newest upload (`last_uploaded_at`) first. Stable order: ties broken by the session ID. No pagination and no filters in v0.2: one response with every session.
- List row: the machine ID and session ID, the upload time, a metadata summary (name, tags and agent version) and the metrics totals: tokens, cost and turns (`null` when none).

## Step 0: Foundations

| # | Task | Done when |
|---|------|-----------|
| 0.1 | Add the session-parser library (`go/v0.1.0`) and golden session files from real sessions in `testdata` | `go.mod` pins the library; a test parses every golden file |
| 0.2 | `MetadataStore` interface (metadata plus metrics totals) | Interface plus a shared contract test suite that any implementation must pass |

## Slice 1: Parse on upload

| # | Task | Done when |
|---|------|-----------|
| 1.1 | SQLite `MetadataStore` (metrics included) plus migration | Passes the 0.2 contract tests |
| 1.2 | `PUT` parses inline and follows the write sequence; rejects with `422` (behavior change, see above) | A valid session stores its metadata; an unparseable one or a header ID mismatch gets `422` and changes no record |
| 1.3 | Re-upload replaces the metadata | A re-upload leaves no stale rows (e.g. removed tags, metrics gone) |

## Slice 2: Read

| # | Task | Done when |
|---|------|-----------|
| 2.1 | `GET /v1/sessions/{machineId}/{sessionId}/meta` | Returns what the upload stored; `404` problem for an unknown session |
| 2.2 | `GET /v1/sessions`: list, no pagination | Newest-upload first in a stable order; each session appears once with its machine ID, session ID, upload time, name, tags, agent version and metrics totals |
| 2.3 | **End-to-end check**: the real pi `/upload` against the server in its container | A real session lands; its meta route and the list show it |

## Not in v0.2

- Job queue, re-parse (including after a library upgrade)
- Tool calls and subagents ([../result-reference-todo.md](../result-reference-todo.md))
- Session relationships (parent session, subagent child sessions): every session stands alone
- A dedicated `MetricsStore` with time series, and the metrics query route
- List pagination and filters
- Transactions or rollback across the write sequence
