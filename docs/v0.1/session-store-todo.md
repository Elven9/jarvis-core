# Session store: upload and raw storage (v0.1)

Goal: the pi `/upload` command can upload a session, overwrite it, and read it back byte-identical.
The sandbox orchestrator is out of scope here (sandbox sessions are for the agents only).
Parsing is not part of this file; see [../v0.2/metadata-storage-todo.md](../v0.2/metadata-storage-todo.md).

## Decisions

### Identity (0.1)

- Global ID = `(type, machineId, sessionId)`. Every upload gets type `pi`; a way to set the type comes later. The type stays in the ID and the primary key, so adding it needs no schema change.
- `type`: in v0.1 the only accepted type is `pi`. Any other type (including `?type=` on the routes) is rejected; an empty `?type=` is rejected too (only a missing parameter defaults to `pi`).
- `machineId`: what the extension accepts, `^[A-Za-z0-9._-]+$`, at most 256 bytes. Not a UUID. `.` and `..` are rejected (the router removes dot segments).
- `sessionId`: whatever pi sends (the session header ID), with minimum checks: not empty, valid UTF-8, no control characters, no `/`, not `.` or `..`, at most 256 bytes. Checked after percent-decoding.
- Both segments arrive URL-encoded; the handler decodes them (`url.PathUnescape`) and then validates. See [tech-stack-decision.md](tech-stack-decision.md#caveats-checked-against-v370) (Fiber caveat 4).
- Everything is case-sensitive (type, `machineId`, `sessionId`): IDs are compared byte for byte and never normalized.
- String form: `type:machineId:sessionId`. `type` and `machineId` cannot contain `:`, so parsing splits on the first two colons; no escaping needed even if `sessionId` contains `:`.
- The same `sessionId` on two machines is two different sessions.

### Storage (0.2, 1.1, 1.2)

- `BlobStore.Put` streams: takes an `io.Reader`, hashes (sha256) while writing to a temp file, then renames into place.
- Filesystem blob layout: `blobs/<first 2 hex chars of the hash>/<full hex hash>`.
- The filesystem blob store keeps the original bytes, uncompressed. It is a temporary dev store and will be replaced.
- Session record: ID parts, current blob hash, size, first/last upload time. No `parse_status` column (P3 decides that).
- Upload history table: every upload appends a row `(session ID, blob hash, size, uploaded_at)`, even when the content is the same as the current one.
- No blob is ever deleted in v0.1: old blobs from overwrites stay until a retention policy exists.

### Upload and read (1.3, 1.5)

- Concurrent or out-of-order uploads: last write wins.
- Body checks: no size limit for now; an empty body is rejected.
- Content type must be text: `text/*` or `application/x-ndjson` (what the extension sends). Anything else, or no content type, is rejected.
- Success: `200` only, empty body. Errors use a 4xx/5xx status with a short plain-text body (the client shows its first 200 chars).
- Read: one route only, `GET /v1/sessions/{machineId}/{sessionId}` (mirrors the PUT path), returns the raw bytes with `content-type: application/x-ndjson`. No `ETag`. (Later moved to `GET /v1/sessions/{machineId}/{sessionId}/raw`; the bare path now answers the parsed session as JSON, see [../TODO.md](../TODO.md#decisions-so-far).) Errors use RFC 9457 `application/problem+json`. No list API yet.

### Still open

- Nothing open. Tech stack: [tech-stack-decision.md](tech-stack-decision.md).

## Step 0: Foundations

| # | Task | Done when |
|---|------|-----------|
| 0.1 | **Session identity**: global ID = `(type, machineId, sessionId)`, type defaulting to `pi`; fix the string form (separator, escaping) and the validation rules | Go ID type with format/parse round-trip tests |
| 0.2 | **Storage interfaces**: `BlobStore` (sha256 content-addressed) and `SessionStore` (the upload record) | Go interfaces plus a shared contract test suite that any implementation must pass |
| 0.3 | **Project skeleton**: package layout, config, HTTP server, logging, test harness, task runner (`just`), two-stage Dockerfile (see [tech-stack-decision.md](tech-stack-decision.md#deployment-container)) | `go test ./...` passes on the empty skeleton; the server starts locally and in its container |

## Slice 1: Upload and raw store

| # | Task | Done when |
|---|------|-----------|
| 1.1 | Local FS `BlobStore` (sha256 path, atomic write, dedupe) | Passes the 0.2 contract tests |
| 1.2 | SQLite `SessionStore` (upsert by ID, current blob hash, size, timestamps, upload history table) plus migrations | Passes the 0.2 contract tests |
| 1.3 | `PUT /v1/sessions/{machineId}/{sessionId}` matching the extension contract exactly: ID validation, no size limit (empty body rejected), content-type check, overwrite, `200` with an empty body, short plain-text error bodies | Contract tests modeled on the extension's `test-server.ts` cases pass |
| 1.4 | **End-to-end check**: the real pi `/upload` against the server running locally in its container (needs the extension moved to `/v1`) | A real session lands, overwrites, and is byte-identical on disk |
| 1.5 | Read API: `GET /v1/sessions/{machineId}/{sessionId}` returns one raw session (list API later) | Can fetch what was uploaded, byte-identical |
