# Result and reference system TODO (separate feature, not v0.1)

Source: [proposal.md](proposal.md)
Builds on the v0.1 session store ([v0.1/session-store-todo.md](v0.1/session-store-todo.md)) and on parsing ([parsing-todo.md](parsing-todo.md)).

## Tool-call references

- Tool-call reference format (global session ID plus tool call ID), in line with the pi-agent tool-ref extension
- Tool-call retrieval API with low token use: args/output, line ranges, char limits (the same ideas as `tool_ref_get`)
- Full tool outputs: per-session `tmp/{uuid}.txt` layout in pi-agent, an upload contract for them, linking them to truncated tool calls, the codemode question

## Object model

- Session / Report / Reference as first-class objects; edges with state `pending`/`linked` (a report can arrive before its session)
- Retention invariants: a referenced tool call keeps its session alive

## Reports

- Fixed JSON format with a format version (agents do not write HTML)
- Versioning v1 → v2 → v3, write lock, conflict handling (TBD)
- Compact form for agents; HTML rendering for the dashboard

## References

- Types: open-source code (permalink, lines, short excerpt), document pages, plain text excerpts, custom (Synology / WorkPlus issues from project instructions)
- Fetch and store the content, not just the link; blob store with sha256 dedupe
- Generated excerpts must be checked, not trusted
