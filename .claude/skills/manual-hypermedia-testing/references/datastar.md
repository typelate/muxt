# Datastar ↔ muxt acceptance checklist

Docs: [Datastar guide](https://data-star.dev/guide/getting_started) ·
[attributes](https://data-star.dev/reference/attributes) ·
[actions](https://data-star.dev/reference/actions) ·
[SSE events](https://data-star.dev/reference/sse_events)

The interface under test: muxt's `datastar(...)` framing (`--use-datastar`)
emits `datastar-patch-elements` / `datastar-patch-signals` SSE frames, decodes
the `signals` argument, and renders `.Actions()` expressions Datastar executes.

## Verify, in order

1. **Boot**: `datastar.js` 200 — an SRI (`integrity=`) mismatch fails
   *silently* except for a console error, so check `list_console_messages`
   before anything else.
2. **Actions render executable**: in the page source the attribute reads
   `data-on:click="@post(&#39;/increment&#39;)"`. `&#39;` is **correct**
   (standard attribute escaping, decoded by the browser); a bug looks like a
   *quoted* expression — `"&#34;@post(...)&#34;"` means the value was a plain
   string, not `template.JS`.
3. **Signals reach the handler**: interacting after `data-bind:name` input
   sends body `{"name":"..."}` with `datastar-request: true` (POST; GET/DELETE
   use the `datastar` query parameter instead).
4. **SSE response contract**: `Content-Type: text/event-stream`,
   `Cache-Control: no-store`; captured body (via `responseFilePath`) frames as
   `event: datastar-patch-elements` + `data: elements <fragment>` (or
   `datastar-patch-signals` + `data: signals {...}`).
5. **Patch applied**: re-snapshot — the fragment replaced the element with the
   matching `id` (no `selector` needed when ids match); datastar-counter shows
   the incremented count and `<p id="greeting">Hello, NAME!</p>`.

Wire-format ground truth is the datastar-go SDK: see
`cmd/muxt/testdata/reference_datastar_conformance.txt` (byte-for-byte).
