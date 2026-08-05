---
name: manual-hypermedia-testing
description: Use when manually verifying muxt-generated handlers against a hypermedia frontend in a real browser — running a docs/examples app, exploring htmx or datastar behavior with the chrome-devtools MCP, or reproducing a frontend-integration bug the txtar archives can't.
---

# Manual Hypermedia Testing

The apps in `docs/examples/` are the acceptance surface between muxt-generated
handlers and the frontend libraries. txtar archives prove the wire; a browser
proves the library actually interprets it. Acceptance checklists per framework:

- **htmx** (htmx-counter :8000, htmx-todo :8002) → [references/htmx.md](references/htmx.md)
- **datastar** (datastar-counter :8001) → [references/datastar.md](references/datastar.md)

## Process

1. `cd docs/examples/<app> && PORT=<port> go run .` via Bash with
   `run_in_background: true`. If templates changed, `go generate ./...` first.
2. Load tools: `ToolSearch "select:mcp__chrome-devtools__new_page,...take_snapshot,...click,...fill,...press_key,...list_network_requests,...get_network_request,...list_console_messages"`.
3. `new_page` at `http://localhost:<port>/` → `take_snapshot` → interact (`click`/`fill`/`press_key`) →
   re-snapshot → assert against the framework checklist: DOM (snapshot),
   wire (`get_network_request`), console (`list_console_messages`).
4. Stop servers with `TaskStop` (and `pkill -f` the compiled binary if `go run`
   leaves a child).

## Chrome MCP tips

| Pitfall | Fix |
|---|---|
| Element uids go stale after every swap | Pass `includeSnapshot: true` on the interacting call; never reuse pre-swap uids |
| `fill` + Enter submits nothing (Enter goes to the native autocomplete popup) | Click the input, press `Escape`, then `Enter` |
| SSE response body truncated/absent inline | `get_network_request` with `responseFilePath:` then Read the file |
| Streaming request looks "stuck" | Normal — SSE holds the connection; assert on the captured body |
| `/favicon.ico` 404 in console/network | Noise; ignore |
| `cd` inside a backgrounded command doesn't carry over to later Bash calls | One `cd <dir> && go run .` server per background task; absolute paths everywhere else |

## Escalation

A behavior difference between an archive's assertions and the browser is a
bug in muxt or in the archive — reproduce it as a txtar archive
(see the testscript-error-assertions skill) before fixing.
