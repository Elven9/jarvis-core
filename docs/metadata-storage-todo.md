# Metadata and time-series metrics storage (after v0.1)

Goal: decide and build where parsed session data and metrics live, behind interfaces that SQLite implements now and stronger stores can implement later.
[parsing-todo.md](parsing-todo.md) writes into this store, so its design comes before the parser.

| # | Task | Done when |
|---|------|-----------|
| M1 | **Requirements**: what parsed data we keep (session summary, entries, branches, ...) and which queries we need. Driven by P4 research | List of entities and queries |
| M2 | Metadata store interface plus SQLite implementation | Passes a shared contract test suite |
| M3 | **Time-series metrics**: which metrics (tokens, cost, durations from the metrics extension), schema, query patterns; SQLite first, interface ready for a time-series DB | Metrics from a real session can be stored and queried over time |
| M4 | Re-parse semantics: an overwrite replaces the old parsed data cleanly (in one transaction) | Re-upload leaves no stale rows |
| M5 | Migrations shared with the session store. **Decided**: one SQLite file, one goose migration sequence (see [v0.1/tech-stack-decision.md](v0.1/tech-stack-decision.md#migrations-goose)) | Migrations run at startup |
