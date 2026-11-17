# Parsed session and session-parser v0.2 (v0.3)

Goal: read every group the session parser supports through the API, and store the new metadata fields.
Builds on [../v0.2/metadata-storage-todo.md](../v0.2/metadata-storage-todo.md).

## Decisions

### Parser

- Library pinned at `go/v0.2.0`: adds scenario and project to `Metadata()`, codemode scripts to `ToolCalls()`, and the `Skills()` and `ProjectInstructions()` groups.

### Routes

- `GET /v1/sessions/{machineId}/{sessionId}`: every parser group of the current upload as JSON (metadata, metrics with samples, tool calls with codemode scripts, subagents, skills, project instructions) and the parser version.
- **Parse every time**: the route parses the stored raw file on each request. Nothing of it is stored, so it always shows what the parser built into the server reads.
- A stored file the parser rejects answers a `422` problem naming the parse error and the parser version.
- `GET /v1/sessions/{machineId}/{sessionId}/raw`: the raw file as `application/x-ndjson` (was the bare path in v0.1 and v0.2).
- `GET /v1/sessions/{machineId}/{sessionId}/meta`: the stored metadata, now with scenario and project.

### Storage

- Migration `00003`: `session_metadata.scenario` and the `session_projects` table (one row per session with a project).
- Metadata stored before the migration keeps no scenario and no project (`null` on the meta route) until the session's next accepted upload. No backfill.

## Tasks

| # | Task | Done when |
|---|------|-----------|
| 3.1 | Bump the library to `go/v0.2.0` | Uploads and the meta route pass unchanged |
| 3.2 | Move the raw download to `.../raw` | The raw tests pass on the new path |
| 3.3 | Parsed session route | Answers every group the parser reads from the upload; `404`/`400`/`422`/`500` problems |
| 3.4 | Store scenario and project; answer them on the meta route | Contract tests round-trip both; rows from before the migration read back without them |

## Open

- Upload validation still runs `Metadata()` and `Metrics()` only, so an upload can be accepted whose other groups fail; the parsed route then answers `422` for it. Whether uploads should check every group is not decided.
