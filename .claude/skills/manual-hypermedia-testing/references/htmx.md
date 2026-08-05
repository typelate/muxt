# htmx ↔ muxt: the generated contract

Docs: [htmx docs](https://htmx.org/docs/) · [attribute/header reference](https://htmx.org/reference/)

## Conceptual map

The coupling is loose and textual: header names and HTML fragments. Each row
is one seam — every acceptance test is picking a row and following it across
the wire in both directions.

| muxt generates | wire | htmx does |
|---|---|---|
| `HTMXTemplateData` request readers (`.HXRequest`, `.HXBoosted`, `.HXTriggerElementID`, `.HXCurrentURL`, …) | [request headers](https://htmx.org/reference/#request_headers) `HX-Request`, `HX-Trigger`, … | sets them on every AJAX request, so templates can branch per trigger or render full pages for direct visits |
| response setters (`.HXRedirect`, `.HXTrigger`, `.HXRetarget`, `.HXReswap`, …) | [response headers](https://htmx.org/reference/#response_headers) `HX-*` | client-side commands: navigate, dispatch events, override target/swap |
| the define body rendered into the buffer | `text/html` fragment body | inserts per `hx-target` / `hx-swap` |
| `.Path()` methods | URLs | authors put them in `hx-get` / `hx-post` attribute values |
| `htmx(sse(...))` — the framing changes only the render type; events keep generic `data:` lines | `text/event-stream` | [SSE extension](https://htmx.org/extensions/sse/) `sse-swap` |

## Systematic exploration

Work the seam from both ends until they meet at the wire:

1. **Generated side.** For each route in the template names, note its framing
   and representation, then find its handler in `template_routes.go`: which
   template-data type does the composite literal construct
   (`HTMXTemplateData` vs plain `TemplateData`), and which `HX*` methods
   exist on it? `grep 'func.*HX' template_routes.go` enumerates the muxt half
   of the contract.
2. **Library side.** Grep the templates for `hx-` attributes and `.HX`
   helper calls. Each one names a row of the map above; for unfamiliar
   attributes read the htmx reference entry to learn what request it issues
   and what response it expects.
3. **Runtime.** Load the page and, for each row you touched, follow one full
   loop in the browser: trigger the attribute → `get_network_request` shows
   the `HX-*` request headers arriving and the fragment or `HX-*` response
   headers returning → re-snapshot shows the swap or command applied. A
   template that branches on a request reader (e.g. `.HXTriggerElementID`)
   deserves one loop per branch; the branch that renders for non-htmx
   requests is exercised by direct navigation (no `HX-*` headers), not by an
   htmx trigger.

## Flag interplay

- `--use-htmx` wraps every route (no per-route opt-out); it is mutually
  exclusive with `--use-datastar`, and `--output-htmx-helpers` is a
  deprecated alias. Mixing framed/unframed means omitting the flag and
  wrapping per route — then unframed routes must NOT have `HX*` helpers
  (that's the safety property, enforced by `muxt check`).
- `--output-htmx-template-data-type` renames the type; the `HX*` methods
  move with it — grep for the configured name, not the default.
- `--output-routes-func-with-path-prefix-param`: the prefix flows through
  `.Path()`, so `hx-*` URLs must include it when the app is mounted under a
  subpath.
- `--output-exported-default-identifiers=false` lowercases default type
  names (`hTMXTemplateData`).
