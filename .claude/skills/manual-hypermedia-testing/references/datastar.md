# Datastar ↔ muxt orientation

Docs: [guide](https://data-star.dev/guide/getting_started) ·
[attributes](https://data-star.dev/reference/attributes) ·
[actions](https://data-star.dev/reference/actions) ·
[SSE events](https://data-star.dev/reference/sse_events)

Tighter than htmx — muxt generates to Datastar v1's *protocol* — but the
coupling is deliberately wire-level only (stdlib-only generated code; the
datastar-go SDK appears solely in the byte-for-byte conformance archive
`cmd/muxt/testdata/reference_datastar_conformance.txt`). Four couplings:
patch-event **protocol**, `.Actions()` **expression language**, signal-store
**state** (Go JSON tags must agree with `data-bind:*` names), and Datastar
**version**.

| muxt generates | wire | Datastar does |
|---|---|---|
| `.Actions()` → `template.JS` `@verb('/url', {opts})` | expression in `data-on:*` / `data-init` | evaluates it, fetches with the signal store attached |
| `signals` argument decode | `datastar-request: true` + JSON body (or `datastar` query param on GET/DELETE) | serializes `data-bind:*` state into every action request |
| `DatastarEventTemplateData.WriteTo` (setters/options → `data:` lines) | `event: datastar-patch-elements` | morphs DOM by `id`, or per selector/mode |
| `marshalJSON(sendX)` senders | `event: datastar-patch-signals` | merges into the client signal store |
| `datastar(marshalJSON(Method(...)))` standalone | `application/json` | plain JSON for non-Datastar consumers; Datastar actions expect SSE |
| shared SSE transport | `text/event-stream`, `Cache-Control: no-store` | applies frames as they flush |

Useful greps: `datastar-patch` and `func.*DatastarActions` in
`template_routes.go`; `data-` in templates.

Runtime gotchas: check the console *first* — a CDN `integrity=` mismatch
fails silently except there. `data-on:click="@post(&#39;/x&#39;)"` is correct
(attribute escaping); a **quoted** expression (`&#34;@post(...)&#34;`) means
a plain string leaked past `template.JS`.

## Flag axes worth varying

- Explicit `datastar(...)` wrappers vs `--use-datastar` (mutually exclusive
  with `--use-htmx`); `signals` and `.Actions()` exist only on framed routes.
- `--output-jsonv2` is client-visible: duplicate-key signal stores get 400
  (v1 takes the last value); `WithJSONOptions` appears.
- `--output-datastar-*-template-data-type` rename the three types; the patch
  option types (`DatastarPatchElementOption`, `DatastarPatchSignalsOption`)
  are fixed.
- `--output-routes-func-with-path-prefix-param` — the prefix must appear in
  `.Actions()` URLs.
