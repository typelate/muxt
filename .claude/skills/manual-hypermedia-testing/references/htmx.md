# htmx ↔ muxt acceptance checklist

Docs: [htmx docs](https://htmx.org/docs/) · [attribute/header reference](https://htmx.org/reference/)

The interface under test: muxt's `htmx(...)` framing (`--use-htmx`) renders
`HTMXTemplateData`, whose `HX*` methods read [request headers](https://htmx.org/reference/#request_headers)
and set [response headers](https://htmx.org/reference/#response_headers) that
htmx acts on.

## Verify, in order

1. **Boot**: page loads, `htmx.min.js` 200 (network), no console errors.
2. **Request headers reach the handler**: interact; on the request
   (`get_network_request`) expect `hx-request: true` plus the specific header
   the template branches on — e.g. htmx-counter's Decrement sends
   `hx-trigger: decrement`, and the `.HXTriggerElementID` branch must pick the
   right arithmetic.
3. **Fragment response**: `Content-Type: text/html`, body is the bare
   fragment (htmx-counter: `<div id='count'>N</div>`), not a full page.
4. **Swap applied**: re-snapshot shows the new value at the `hx-target`
   element; count matches the click sequence (+1 +1 −1 → 1).
5. **Template-set response headers arrive** (routes using `.HXRedirect`,
   `.HXTrigger`, `.HXRetarget`, …): assert the `HX-*` response header on the
   wire and the client behavior it commands.
6. **Non-htmx fallback**: fetch a fragment route directly (`navigate_page`) —
   templates branching on `.HXRequest` must render the full-page variant.

htmx-todo exercises the richer surface: form post with
`hx-on::after-request` reset (input empties after add), `beforeend` swap
(item appends), toggle → items-left recount, `response-targets` extension.
