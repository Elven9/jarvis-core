# Parsed session and session-parser v0.2 (v0.3)

Goal: read every group the session parser supports through the API, and store the new metadata fields.
Builds on [../v0.2/metadata-storage-todo.md](../v0.2/metadata-storage-todo.md).

## Decisions

### Parser

- Library pinned at `go/v0.2.0`: adds scenario and project to `Metadata()`, codemode scripts to `ToolCalls()`, and the `Skills()` and `ProjectInstructions()` groups.

### Upload (strict)

- An upload is parsed with every parser group, not only the stored ones. Any group rejecting the file rejects the upload with `422` (behavior change from v0.2, which checked `Metadata()` and `Metrics()` only).
- So every accepted session parses on the parsed session route with the parser that accepted it.

### Routes

- `GET /v1/sessions/{machineId}/{sessionId}`: every parser group of the current upload as JSON (metadata, metrics with samples, tool calls with codemode scripts, subagents, skills, project instructions) and the parser version.
- **Parse every time**: the route parses the stored raw file on each request. Nothing of it is stored, so it always shows what the parser built into the server reads.
- A stored file the parser rejects answers a `422` problem naming the parse error and the parser version. With strict uploads this only happens for files accepted by an older, more lenient server or parser.
- `GET /v1/sessions/{machineId}/{sessionId}/raw`: the raw file as `application/x-ndjson` (was the bare path in v0.1 and v0.2).
- `GET /v1/sessions/{machineId}/{sessionId}/meta`: the stored metadata, now with scenario and project.
- `GET /v1/sessions`: each row adds `scenario` and `project` (the project's name), `null` when absent.

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
| 3.5 | Golden files from recent sessions | Every golden parses in every group; the set covers scenario, project, codemode scripts, subagent results, skill loads, project instructions and their reads |
| 3.6 | Strict upload | An upload any group rejects gets `422` and changes no record |
| 3.7 | Scenario and project in the list | Each row has both; `null` when absent |

## Not covered

- No real session has a remote project yet, so no golden file has one; hand-written fixtures cover it.
