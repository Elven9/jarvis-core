# v0.1 tech stack decisions

Decisions on how the v0.1 server is built. Feature decisions live in the todo files (e.g. [session-store-todo.md](session-store-todo.md)); the index is [../TODO.md](../TODO.md).

## API style: REST

The server is a RESTful HTTP server.

- Resources map to URLs: sessions now, reports, references and tool calls later.
- The upload contract is already REST: `PUT /sessions/{machineId}/{sessionId}` replaces the stored session and is idempotent; `GET` on the same path reads it back.
- Raw session bytes go over the wire as is; metadata uses JSON.

Open:

- API version prefix: the extension calls `/sessions/...` with no prefix, so that route stays unprefixed. No prefix at all, or `/v1` for new routes only?
- Where the session type goes in URLs once there is more than `pi` (e.g. `/sessions/{type}/{machineId}/{sessionId}`, keeping the current path as the `pi` form).
- Actions that are not resources (e.g. manual re-parse): model as creating a resource, e.g. `POST /sessions/{m}/{s}/parse-jobs`?
- Error format for future JSON routes (`application/problem+json`, RFC 9457?). The upload route keeps short plain-text errors because the client shows them.

## HTTP framework: Fiber v3

[gofiber/fiber](https://github.com/gofiber/fiber) v3 (v3.7.0 when chosen, 2026-11; MIT; needs Go 1.26+).

- Built on `fasthttp`, not `net/http`. `net/http` handlers and middleware can still be mounted through the `adaptor` middleware.
- Built-in middleware we are likely to use: `logger`, `recover`, `requestid`, `healthcheck`.
- Tests: `app.Test(*http.Request)` takes a standard request, so contract tests are plain `httptest` code.

### Config

```go
fiber.New(fiber.Config{
    CaseSensitive:     true, // IDs and routes are case-sensitive
    StrictRouting:     true, // /sessions/a/b/ is not /sessions/a/b
    StreamRequestBody: true, // required by the upload route: streaming, no size cap
    Immutable:         true, // ctx values are safe to keep after the handler
    // UnescapePath stays false (default); handlers decode params themselves
})
```

### Caveats (checked against v3.7.0)

1. **Body limit.** `BodyLimit` defaults to 4 MB and 0 or a negative value falls back to that default, so "no limit" cannot be set directly. Without streaming a 10 MB upload is rejected.
2. **`StreamRequestBody` is required for the upload route.** With it, a body larger than `BodyLimit` is streamed to the handler with no cap (10 MB tested). It is an app-wide setting, not per route.
   - A body smaller than `BodyLimit` is still buffered, and then `c.Req().BodyStream()` is nil. The upload handler must use `BodyStream()` when it is non-nil and `bytes.NewReader(c.Body())` otherwise.
   - `Body()`, form values and binders still apply `BodyLimit`; only `BodyStream()` reads past it.
3. **Case sensitivity.** `CaseSensitive` defaults to false (`/SESSIONS/...` would match `/sessions/...`). Param values keep their original case either way. We set it to true to match the case-sensitive ID decision.
4. **Params are not percent-decoded.** The extension URL-encodes both segments; by default `c.Params` returns them still encoded (`Ab%3Acd%20e`).
   - `UnescapePath: true` decodes before routing, so an encoded `/` (`%2F`) would split into a new path segment and the route would not match. We keep it false and decode each param with `url.PathUnescape` in the handler, then validate it.
   - The router always normalizes the path (RFC 3986 6.2.2): escapes of unreserved characters are decoded, and `.`/`..` segments are removed. So an ID of `.` or `..` never reaches the handler as a param.
5. **Zero-copy context values.** By default `Params`, headers, query values and `Body()` point into a buffer the next request overwrites. We set `Immutable: true` (personal, single-user server; the extra allocations do not matter), so values can be kept without copying.
6. **No HTTP/2** in `fasthttp`. Fine behind a proxy or Cloudflare, which talk HTTP/1.1 to the origin (see [Future work: deployment](#future-work-deployment)).

## SQLite driver: mattn/go-sqlite3 (cgo)

[mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) (v1.14.53 when chosen, 2026-11; MIT). Wraps the C SQLite amalgamation (bundled, SQLite 3.54.1) through cgo; `database/sql` driver named `sqlite3`.

Why: the real SQLite C code, including its own file locking and VFS; the most used Go SQLite driver. The cost of cgo is paid inside the builder container (see Deployment).

Checked with `CGO_ENABLED=1` on macOS: WAL and foreign keys set from the DSN work; cold build about 17-18 s, 7.3 MB binary.

Caveats:

1. **cgo is required**: `CGO_ENABLED=1` and a C compiler (gcc) at build time, also for `go test`. Cross-compiling needs a C cross compiler: a plain `GOOS=linux go build` on macOS fails. Linux binaries are built in the builder container; on macOS, native builds work with the Xcode command line tools.
2. **Transactions are deferred by default** (`BEGIN`). A transaction that reads and then writes fails with `database is locked` under contention, even with a busy timeout: 8 goroutines x 100 read-then-write transactions gave 656 of 800 failures with the default, 0 with `_txlock=immediate`. Always set `_txlock=immediate`.
3. **FTS5 needs a build tag**: without `-tags sqlite_fts5`, `fts5` is `no such module`. FTS3 and R*Tree are compiled in by default. Add the tag once search lands.
4. Settings use mattn's own DSN keys (`_journal_mode`, `_foreign_keys`, `_busy_timeout`, `_txlock`, ...), not `_pragma=...`. Default busy timeout is 5 s.
5. Mostly one maintainer (mattn 719 commits, next 97), but a large user base (9.3k stars) and regular releases that follow upstream SQLite.
6. Chinese full-text search (later): FTS5 `trigram` needs 3+ characters, so 2-character words do not match (tested on another driver; same SQLite tokenizer). A custom tokenizer would be C (an SQLite extension, or ICU through `-tags sqlite_icu`).

Setup: `file:<path>?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000&_txlock=immediate`.

Considered: `ncruces/go-sqlite3` (no cgo, SQLite translated to Go; pre-1.0, its own Go VFS) and `modernc.org/sqlite` (no cgo, transpiled C). Not chosen; both stay possible behind `database/sql`.

## Migrations: goose

[pressly/goose](https://github.com/pressly/goose) v3 (v3.30.0 when chosen, 2026-11; MIT). One SQLite file, one migration sequence (M5).

Setup (minimal):

- SQL files in `migrations/`, sequential versions (`00001_init.sql`, `goose create -s`), embedded with `//go:embed`.
- At startup, before the HTTP server listens: `goose.NewProvider(goose.DialectSQLite3, db, fsys)` then `Up(ctx)`. Any error stops startup.
- Startup check: refuse to start when the database version is higher than the newest embedded migration (goose does not check this).
- No `Down` sections; undo is restoring a backup. No goose CLI for now.
- Logging through `WithSlog` if we use `slog`.

Workflow:

- Before the first release: only `00001_init.sql`, edited freely; delete the dev database after each edit.
- From the first release on: a shipped migration file is never edited. Every schema change is a new, higher-numbered file.

How it works (checked with v3.30.0 and mattn/go-sqlite3):

- Each file runs in its own transaction; a failing file is rolled back completely and the version stays. With several pending files, the database ends at the last one that succeeded.
- `goose_db_version` (`id, version_id, is_applied, tstamp`) is an append-only log: a version 0 row when created, then one row per applied file. The current version is the highest applied one.
- A new file numbered below the current version is an error (`missing (out-of-order) migration`) unless `WithAllowOutofOrder` is set.
- `-- +goose NO TRANSACTION` runs a file outside a transaction (needed for SQLite table rebuilds, since `PRAGMA foreign_keys` cannot change inside a transaction). Such a file is not rolled back on failure: keep it small and back up before deploying.

Caveats:

1. No checksums: editing a file that already ran is silently ignored. The "never edit a shipped file" rule is ours to keep.
2. Older code on a newer database starts silently, hence the startup check above.
3. No migration lock for SQLite (lockers exist only for Postgres and MySQL). Fine with one server instance; two containers must never share the volume.
4. Small footprint: of the 90 modules in goose's `go.mod`, only `mfridman/interpolate`, `sethvargo/go-retry`, `go.uber.org/multierr` and `x/sync` are compiled in.

## Logging: slog, JSON, stdout

- `log/slog` everywhere: our code, goose (`WithSlog`), and Fiber.
- JSON everywhere (`slog.NewJSONHandler`), locally and in the container.
- Everything goes to stdout by default. Rotation and retention are the container runtime's job (e.g. the `json-file` log driver with `max-size`).
- Access log: our own middleware (not Fiber's `logger`), writing one `slog` record per request. Fields: method, path, status, latency, response size, request ID (`requestid` middleware, `requestid.FromContext`), error.
  - Registered first, so it wraps everything (including `recover`).
  - When `c.Next()` returns an error, the status is not set yet: Fiber's error handler normally runs after the whole chain returns. The middleware must call the app's error handler itself (falling back to a bare 500 if that fails), log the final status, and return nil so the error is not handled twice. This is what Fiber's `logger` does.
  - Streamed responses (`SendStream` on GET) are written after the handler returns: latency covers the handler, not the transfer, and the size is only known when `Content-Length` was set.
- Never log request bodies (whole sessions; tool output can contain secrets) or headers (the extension can send Cloudflare Access service-token headers). Log `machineId` and `sessionId` as separate fields.

Fiber caveats (checked against v3.7.0):

1. Fiber's `log` package has no slog backend, and `gofiber/contrib` only has zap and zerolog adapters. To get Fiber's own log lines as JSON we write a small adapter that implements `log.AllLogger` on top of slog and install it with `log.SetLogger`.
2. Fiber's `logger` middleware is not used: its default output is a text template and structured output needs `LoggerFunc` plus its `Data` type; our own middleware is about as short and fully ours.
3. Fiber prints a startup banner that is not JSON; disable it with `ListenConfig.DisableStartupMessage`.

## Configuration: environment variables, standard library

- Config comes from environment variables only, read with the standard library (`os.LookupEnv`) into one `Config` struct at startup. No config file, no flags.
- Switch to [spf13/viper](https://github.com/spf13/viper) once the configuration gets more complex (files, nesting, many keys). Notes from checking v1.21.0 for that switch: use `viper.New()` (not the global), `SetDefault` or `BindEnv` every key (`AutomaticEnv` alone is ignored by `Unmarshal`), always `UnmarshalExact` (plain `Unmarshal` silently ignores typos), keys are lowercased, no validation, 13 modules compiled in, last release 2025-09.

Rules:

- All variables share the prefix `JARVIS_` (working name).
- Every setting has a default, except the ones marked required.
- Invalid values (bad duration, unknown log level, ...) stop startup with an error naming the variable.
- An unknown `JARVIS_*` variable stops startup (catches typos).
- The effective config is logged once at startup.

Settings (initial):

| Variable | Default | Meaning |
|----------|---------|---------|
| `JARVIS_ADDR` | `:8080` | Listen address |
| `JARVIS_DATA_DIR` | `./data` (`/data` in the container image) | SQLite file and blobs |
| `JARVIS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `JARVIS_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown limit (Go duration) |

## Deployment: container

The server runs as a container image, built in two stages.

- **Base image**: `ubuntu:26.10` for both stages (Ubuntu 26.10 "stonking"). A version tag: it still moves with Ubuntu's image updates, so builds are not bit-for-bit reproducible. 26.10 is an interim release, not LTS (`26.04` is the LTS), with about 9 months of support.
- **Builder stage**: installs a C compiler (`gcc`, `libc6-dev`) and the Go toolchain at the version in `go.mod` (official tarball, checksum verified, not the Ubuntu package, which may lag). Builds the server with `CGO_ENABLED=1`.
- **Runner stage**: only the server binary is copied in; runs as a non-root user; `JARVIS_DATA_DIR=/data`, a mounted volume.
- Builder and runner use the same base image, so the cgo binary runs against the same glibc it was built with. Changing one base image means changing both.

### Data directory

Everything the server stores lives under `JARVIS_DATA_DIR` (for now):

- `jarvis.db` (plus SQLite's `jarvis.db-wal` and `jarvis.db-shm`): the one SQLite file.
- `blobs/`: the filesystem blob store (layout inside is task 1.1).
- `tmp/`: temp files for atomic blob writes. Same filesystem as `blobs/`, so the final rename is atomic.

### Architecture

- The image is built for the build host's own architecture: `linux/arm64` on the Mac (Docker Desktop's Linux VM), `linux/amd64` on an Intel/AMD Linux server. `docker build` does this by default.
- Native builds keep cgo simple: no C cross compiler, no emulated (QEMU) build.
- An image only runs on hosts of the architecture it was built on: build on (or for) the machine that runs it.

### Network

- Plain HTTP, no TLS in the server. It listens on `JARVIS_ADDR`; the port and bind address are chosen when the container is run.

## Future work: deployment

Not part of v0.1; collected here so the notes are not lost.

- **Reverse proxy and Cloudflare Tunnel/Access** in front of the server. TLS ends at Cloudflare; Access does the auth. The container port should then be reachable only by the proxy.
- **Client IP**: the server will see the proxy's address. Fiber's `TrustProxy` stays off unless real client IPs are needed; then trust only the proxy.
- **Body size limits in front of the server** apply even though the server has none: nginx `client_max_body_size` defaults to 1 MB; Cloudflare caps request bodies by plan (about 100 MB on Free/Pro, unverified). The proxy needs a large body limit and long enough timeouts.
- **Base image upgrade**: `ubuntu:26.10` support ends about 9 months after release; move both stages to the next release around mid-2027 (or to an LTS).
- **Multi-arch image** (`docker buildx`, arm64 + amd64), if one image has to run on both.
- **Log driver**: configure rotation in the container runtime (e.g. `json-file` with `max-size`).
- **Backups**: back up `JARVIS_DATA_DIR`; SQLite through its backup API or with the server stopped, never by copying a live `jarvis.db`.
