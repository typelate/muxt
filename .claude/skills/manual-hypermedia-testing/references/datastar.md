# Datastar ↔ muxt: the generated contract

Docs: [Datastar guide](https://data-star.dev/guide/getting_started) ·
[attributes](https://data-star.dev/reference/attributes) ·
[actions](https://data-star.dev/reference/actions) ·
[SSE events](https://data-star.dev/reference/sse_events)

## How the two are coupled

The coupling is tighter than htmx's header names — muxt generates to
Datastar's *protocol* — but it is deliberately wire-level only:

- **Protocol coupling.** The generated event marshaler emits exactly the
  `datastar-patch-elements` / `datastar-patch-signals` SSE frames (`data:
  elements|signals|selector|mode|…` line prefixes) Datastar v1 consumes.
  `cmd/muxt/testdata/reference_datastar_conformance.txt` pins this
  byte-for-byte against the datastar-go SDK — the SDK is a test-time
  reference only; generated code stays stdlib-only, so there is no
  type-level or dependency coupling.
- **Expression-language coupling.** `.Actions()` methods emit Datastar
  expressions (`@post('/users/42', {openWhenHidden: true})`) as
  `template.JS`, injection-proofed by the builder (percent-encoded segments,
  JS-string escaping). muxt knows the route table; Datastar evaluates the
  expression.
- **State coupling.** Datastar sends its whole signal store with every
  action (`datastar-request: true`; JSON body on POST-like methods, the
  `datastar` query parameter on GET/DELETE) and the generated `signals`
  binding decodes it into the method parameter's type — the Go struct's
  JSON tags and the template's `data-bind:*` names must agree.
- **Version coupling.** All three of the above target Datastar v1 (colon
  attribute forms `data-on:click`, v1 event names). A Datastar major bump is
  a muxt marshaler/builder change, caught first by the conformance archive.

## Conceptual map

| muxt generates | wire | Datastar does |
|---|---|---|
| `.Actions()` → `template.JS` `@verb('/url', {opts})` | expression inside `data-on:*` / `data-init` | evaluates it, issues the fetch with the signal store attached |
| `signals` argument decode | `datastar-request: true` + JSON body or `datastar` query param | serializes `data-bind:*` state into every action request |
| render senders through `DatastarEventTemplateData.WriteTo` (template setters `.Selector`/`.Mode`/… and `WithSelector`/`WithMode`/… options → `data:` lines) | `event: datastar-patch-elements` | morphs the DOM — by element `id` when no selector, else per selector/mode |
| `marshalJSON(sendX)` senders | `event: datastar-patch-signals`, `data: signals {...}` | merges into the client signal store |
| `datastar(marshalJSON(Method(...)))` standalone | `application/json` body | plain JSON — for non-Datastar consumers; Datastar actions themselves expect SSE responses |
| SSE transport (shared) | `text/event-stream`, `Cache-Control: no-store` | holds the connection, applies each frame as it flushes |

## Systematic exploration

1. **Generated side.** Per route, identify framing/representation from the
   template name, then in `template_routes.go`: which of the three
   template-data types the handler constructs
   (`DatastarTemplateData` render / `DatastarEventTemplateData` event /
   `DatastarSignalsTemplateData` standalone signals), and read the
   `WriteTo` method — it *is* the protocol implementation. `grep
   'datastar-patch' template_routes.go` finds the frame emitters; `grep
   'func.*DatastarActions'` enumerates the action builders.
2. **Library side.** Grep templates for `data-` attributes and `.Actions`
   calls; match each to its attributes/actions reference entry to learn what
   request it triggers and which frame types it can apply.
3. **Runtime.** Check the console *first* — an `integrity=` (SRI) mismatch
   on the CDN script fails silently except for a console error, and then
   nothing works. Confirm actions render *executable*:
   `data-on:click="@post(&#39;/x&#39;)"` is correct (`&#39;` is standard
   attribute escaping the browser decodes); a **quoted** expression
   (`&#34;@post(...)&#34;`) means a plain string leaked past `template.JS`.
   Then follow one loop per map row: trigger → request (signals body/query
   present?) → captured SSE body (`responseFilePath:`) shows the expected
   frames → re-snapshot shows the morph or updated signal-driven DOM.

## Flag interplay

- `--use-datastar` wraps every route; mutually exclusive with `--use-htmx`.
  The `signals` argument and `.Actions()` exist only on datastar-framed
  routes.
- `--output-jsonv2` changes *client-visible* behavior: signals decode and
  patch-signals marshal use `encoding/json/v2`, so a signal store with
  duplicate keys is rejected (400) where v1 took the last value, and
  `WithJSONOptions` appears on the signals option type.
- `--output-datastar-*-template-data-type` rename the three types; grep the
  configured names.
- `--output-routes-func-with-path-prefix-param`: the prefix flows into
  `.Actions()` URLs — under a subpath, verify `@verb('/prefix/...')`.
- `--output-sse-event-option-type` renames only the *generic* option type;
  the Datastar option types (`DatastarPatchElementOption`,
  `DatastarPatchSignalsOption`) have fixed names, unaffected even by
  `--output-exported-default-identifiers=false` (which lowercases the
  datastar template-data defaults like the other default type names).
