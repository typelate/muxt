---
name: manual-hypermedia-testing
description: Use when manually verifying muxt-generated handlers against a hypermedia frontend in a real browser — reviewing a docs/examples app's integration, exploring htmx or datastar behavior with the chrome-devtools MCP, varying generate/check flags, or reproducing a frontend-integration bug the txtar archives can't.
---

# Manual Hypermedia Testing

Systematically review the integration between muxt-generated handlers and the
frontend library using the apps in `docs/examples/`. The per-framework
references are orientation, not scripts — spend the context window exploring
the actual generated code, templates, and wire traffic.

| Example | Port | Framework | Reference |
|---|---|---|---|
| htmx-counter, htmx-todo | 8000, 8002 | `--use-htmx` | [references/htmx.md](references/htmx.md) |
| datastar-counter | 8001 | `--use-datastar` | [references/datastar.md](references/datastar.md) |

## Review method

Work each seam from both ends until they meet at the wire:

1. **Read both halves.** The example's template names + `template_routes.go`
   (what muxt generated) against its templates' `hx-*` / `data-*` attributes
   (what the library will do) — the reference's map orients you.
2. **Run and follow loops.** Start the app, then for each seam you touched:
   trigger it in the browser → inspect the request/response with
   `get_network_request` → re-snapshot to confirm the DOM effect. One loop
   per template branch.
3. **Vary the flags.** Copy the example into a scratch module and regenerate
   with different flag combinations (axes suggested per reference); re-check,
   re-run, re-verify the loops that the flag should (or should not) change.

## Flag-variation scratch copy

Examples are packages in the muxt module; a scratch copy needs its own
`go.mod` (they are stdlib-only), and muxt must run **from this repo** (its
module path resolves nowhere else) with `-C` pointing at the copy:

```bash
cp -r docs/examples/<app> /tmp/<app>
(cd /tmp/<app> && go mod init scratch)
# from the repo root:
go run github.com/typelate/muxt -C /tmp/<app> generate <flags…>
go run github.com/typelate/muxt -C /tmp/<app> check
go build -C /tmp/<app> -o build . && PORT=<port> /tmp/<app>/build   # then browser-verify
```

## Mechanics

- Servers: build then run the binary (`go build -o build ./docs/examples/<app> && PORT=<port> ./build`)
  in Bash with `run_in_background: true`, one per task — `TaskStop` then
  kills the server directly (`go run` leaves an orphaned child).
- Browser: `ToolSearch "select:mcp__chrome-devtools__new_page,...take_snapshot,...click,...fill,...press_key,...list_network_requests,...get_network_request,...list_console_messages"`,
  then `new_page` at `http://localhost:<port>/`.

| Pitfall | Fix |
|---|---|
| Element uids go stale after every swap | `includeSnapshot: true` on the interacting call |
| `fill` + Enter submits nothing (native autocomplete popup ate it) | Click input, `Escape`, then `Enter` |
| SSE body absent inline / request looks "stuck" | `get_network_request` with `responseFilePath:`, Read the file; SSE holds the connection |
| `/favicon.ico` 404 | Noise |
| `cd` in a backgrounded command doesn't persist | Absolute paths in later commands |

## Escalation

A difference between archive assertions and browser behavior is a bug in
muxt or the archive — reproduce it as a txtar archive (see
testscript-error-assertions) before fixing.
