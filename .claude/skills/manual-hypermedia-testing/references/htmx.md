# htmx ↔ muxt orientation

Docs: [htmx docs](https://htmx.org/docs/) · [attribute/header reference](https://htmx.org/reference/)

Loose, textual coupling: header names and HTML fragments. Each map row is one
seam to follow across the wire.

| muxt generates | wire | htmx does |
|---|---|---|
| `HTMXTemplateData` request readers (`.HXRequest`, `.HXTriggerElementID`, …) | `HX-*` request headers | sets them on every AJAX request; templates branch on them (direct navigation exercises the no-htmx branch) |
| response setters (`.HXRedirect`, `.HXTrigger`, `.HXRetarget`, …) | `HX-*` response headers | client-side commands: navigate, dispatch events, override target/swap |
| define body → buffer | `text/html` fragment | inserts per `hx-target` / `hx-swap` |
| `.Path()` methods | URLs | values for `hx-get` / `hx-post` |
| `htmx(sse(...))` (render type changes; events stay generic `data:` lines) | `text/event-stream` | [SSE extension](https://htmx.org/extensions/sse/) `sse-swap` |

Useful greps: `func.*HX` in `template_routes.go`; `hx-` in templates.

## Flag axes worth varying

- Explicit `htmx(...)` wrappers vs `--use-htmx` (and its deprecated alias
  `--output-htmx-helpers`); mixed framed/unframed files — unframed routes
  must lack `HX*` helpers (`muxt check` enforces).
- `--output-htmx-template-data-type` / `--output-exported-default-identifiers=false`
  rename the types your greps target.
- `--output-routes-func-with-path-prefix-param` — the prefix must appear in
  `.Path()`-derived `hx-*` URLs.
