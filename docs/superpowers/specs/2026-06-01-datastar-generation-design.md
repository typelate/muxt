# Datastar generation support for muxt

**Status:** Draft for review
**Date:** 2026-06-01
**References:**
- Datastar SSE events — https://data-star.dev/reference/sse_events
- Datastar backend actions / response handling — https://data-star.dev/reference/actions#response-handling
- Official Go SDK (wire-format ground truth) — https://github.com/starfederation/datastar-go

## Summary

Add first-class [Datastar](https://data-star.dev) support to the muxt code
generator, parallel to the existing `sse` and htmx-helper features. A new
`--use-datastar` mode lets a receiver method emit Datastar-framed responses
(element patches, signal patches, scripts) and lets templates render Datastar
backend-action expressions (`@get('/x', {…})`) with a fluent options API. The
mode flag is shared between `generate` and `check` so the type checker stays
aware of the datastar-specific template-data methods.

This is one spec implemented as a single sequenced plan / PR.

## Goals

- A `--use-datastar` mode that swaps the generated template-data types and adds
  Datastar helpers, mutually exclusive with htmx mode.
- Reserved callback args for the three non-fragment Datastar response
  representations: `elements`, `signal`, `script`.
- A `DatastarEventTemplateData` type that frames template output as a
  `datastar-patch-elements` SSE event, with chainable metadata setters.
- Context-sensitive `signal` behavior: standalone JSON body vs inline
  `datastar-patch-signals` event when the route streams.
- `signal` JSON marshaling that defaults to `encoding/json` and opts into
  `encoding/json/v2` `MarshalWrite` (into the pooled buffer) via `--output-jsonv2`.
- A `.Actions()` template accessor (Datastar mode only) rendering
  `@verb('/path', {options})` with a fluent builder.
- `check` honors the mode flag and type-checks templates against the correct
  template-data type.

## Non-goals (future follow-up)

- Datastar-specific *semantic* validations in `check` (mode/namespace value
  sets, JSON-encodability of signal results, "selector required for mode X").
  This spec only makes `check` *type-aware*.
- Request-side helpers for reading incoming Datastar signals from the request
  body. Out of scope for v1.
- Streaming routes whose only streaming output is signals (no `elements`). See
  "Open points" — v1 determines "is a stream" from the presence of `elements`.

## Decisions (locked during brainstorming)

| Decision | Choice |
| --- | --- |
| Flag shape | `--use-htmx` / `--use-datastar`, shared on `generate` + `check`, mutually exclusive. `--output-htmx-helpers` becomes a deprecated alias of `--use-htmx`. No `--output-datastar-helpers`. |
| Type names | `TemplateData` → `DatastarTemplateData`; `SSETemplateData` → `DatastarEventTemplateData` (both only under `--use-datastar`). |
| Element-patch arg | `elements` / `elementsX` (gated behind `--use-datastar`). |
| Signal-patch arg | `signal` / `signalX`. |
| Script arg | `script` / `scriptX`. |
| Representation selection | **Fixed per route by declared args** (no runtime Accept negotiation). |
| `.Actions()` | Datastar mode only; htmx mode keeps `Path()`-only. |
| jsonv2 | Opt-in via `--output-jsonv2` (off by default → `encoding/json`); no GOEXPERIMENT auto-detection. |
| Scope | One spec, one sequenced plan / PR. |
| check scope | Mode-awareness (type-checking) only. |

## Wire format (ground truth)

Per the Datastar v1 protocol, an SSE field is `data: <token> <value>` — a **space**
after the token, not a colon. There are exactly two event types.

### `datastar-patch-elements`

```
event: datastar-patch-elements
data: selector #target
data: mode inner
data: namespace svg
data: useViewTransition true
data: elements <div>line one
data: elements line two</div>

```

- `mode` default `outer` — omitted when `outer`.
- `useViewTransition` default `false` — omitted when false.
- `namespace` omitted unless set (`svg` / `mathml`).
- `selector` omitted unless set (not required for `outer`/`replace`).
- Each line of the rendered HTML is emitted as its own `data: elements <line>`.

### `datastar-patch-signals`

```
event: datastar-patch-signals
data: signals {"count":5}
data: onlyIfMissing true

```

- `onlyIfMissing` default `false` — omitted when false.

## Architecture

The change follows the existing generator pipeline:

```
Template name (route + method call with reserved args)
  -> Parser (internal/muxt)          : recognize elements/signal/script args (datastar mode)
  -> Type checker (analysis + check) : mode-aware template-data model
  -> Generator (internal/generate)   : emit Datastar types, handlers, Actions()
```

### Mode plumbing

- `generate.RoutesFileConfiguration`: keep `HTMXHelpers bool`; add `Datastar bool`.
- `analysis.CheckConfiguration`: add `HTMXHelpers bool` and `Datastar bool`.
- CLI: register `--use-htmx` / `--use-datastar` on **both** `generate` and
  `check` via a shared registration helper. Enforce mutual exclusion in the
  command `RunE` (or `PersistentPreRunE`) with a clear error. Wire
  `--output-htmx-helpers` to the same `HTMXHelpers` target and mark it deprecated
  via the existing `markDeprecated` helper.

### Reserved args and representation selection

Recognized **only when `--use-datastar` is set** (otherwise the parser reports an
unknown identifier, mirroring how unknown call args fail today). Like `sse`,
each family supports prefixed variants (`elementsClock`, `signalCount`) and may
appear multiple times.

Representation is **fixed at generation time** from the declared args:

| Declared args (besides ctx/path/etc.) | Response | Content-Type |
| --- | --- | --- |
| any `elements` present | streaming patch-elements (+ inline `signal` events) | `text/event-stream` |
| exactly one `signal`, no `elements` | JSON body | `application/json` |
| exactly one `script`, no `elements`/`signal` | script body | `text/javascript` |
| none of the above | rendered template (`.Result`) | `text/html` |

"Is a stream" is determined by the presence of an `elements` arg. When a route
streams, every `signal` callback emits an inline `datastar-patch-signals` event
instead of writing a JSON body.

### Callback signatures (synthesized when the receiver method is undefined)

Mirrors the existing `sse` synthesis (`func(any) error`):

- `elements` / `elementsX`: `func(data T) error`; synthesized `func(any) error`.
  Renders the **same-named template** into a `DatastarEventTemplateData[R,T]`
  (`.Result` is `data`). Output framed as `data: elements <line>`.
- `signal` / `signalX`: `func(data T, onlyIfMissing bool) error`; synthesized
  `func(any, bool) error`. **No template** — JSON-marshals `data`. Standalone →
  `application/json` body; streaming → `datastar-patch-signals` event with
  `data: onlyIfMissing true` only when true.
- `script` / `scriptX`: `func(data T) error`; synthesized `func(any) error`.
  Renders the same-named template producing JavaScript text, written as the
  `text/javascript` response body. (Script is a non-streaming representation;
  to inject a script mid-stream, append a `<script>` via `elements`.)

## Generated types

### `DatastarEventTemplateData[R, T]` (replaces `SSETemplateData` in datastar mode)

Built the same way as `SSETemplateData` (AST construction in a new
`internal/generate/datastar_event_template_data.go`), retaining `Receiver()`,
`Result()`, `Request()`, `Err()`, `Path()`, `String()`. Differences:

- Struct fields for `selector`, `mode`, `namespace *string` and a
  `useViewTransition *bool` in place of `event`/`id`/`retry`.
- Chainable setters used from inside the element template:
  - `.Selector(string) *DatastarEventTemplateData[R,T]`
  - `.Mode(string) *DatastarEventTemplateData[R,T]`
  - `.Namespace(string) *DatastarEventTemplateData[R,T]`
  - `.UseViewTransition(bool) *DatastarEventTemplateData[R,T]`
- `WriteTo` emits `event: datastar-patch-elements`, then metadata lines (omitting
  defaults per the wire-format rules), then each buffered template line as
  `data: elements <line>`, then the blank terminator. CRLF normalization is kept
  from `SSETemplateData`.

`mode`/`namespace` are plain strings in v1 (no generated enum constants; semantic
validation is a follow-up).

### `DatastarTemplateData[R, T]` (replaces `TemplateData` in datastar mode)

Everything `TemplateData` has, plus an `.Actions()` accessor (below). HTMX mode
does **not** get `.Actions()` — it keeps `TemplateData` + HX* helpers + `.Path()`
only.

### `.Actions()` and the `DatastarActions` / `DatastarAction` types

Parallel to `Path()` / `TemplateRoutePaths` (new
`internal/generate/datastar_actions.go`):

- `.Actions()` returns a `DatastarActions` value carrying `pathsPrefix`.
- One method per route, named like the Path methods, taking the same path
  parameters. The **HTTP verb is inferred from the route's method**
  (`GET`→`@get`, `POST`→`@post`, `PUT`→`@put`, `PATCH`→`@patch`, `DELETE`→`@delete`).
- Each method returns a `DatastarAction` builder whose `String()` renders
  `@verb('/resolved/path', {options})`, options omitted when empty.
- Fluent setters covering the documented action options:
  - `.OpenWhenHidden(bool)` and `.ContentType(string)` — the named priorities.
  - `.Selector(string)`, `.Headers(...)`, `.FilterSignals(...)`,
    `.RequestCancellation(string)`, `.Retry(string)`, `.RetryInterval(int)`,
    `.RetryScaler(float)`, `.RetryMaxWait(int)`, `.RetryMaxCount(int)`.
  - Implemented behind one builder pattern; only set options are serialized.

Templates use it via the `.JS()` terminal, which returns `template.JS` so the
expression renders verbatim inside Datastar `data-on-*` event attributes (which
`html/template` parses as a JavaScript context and would otherwise JSON-wrap and
escape a plain `Stringer`):

```gotemplate
<button data-on-click="{{ (.Actions.UpdateUser .ID).JS }}">save</button>
<a data-on-click="{{ ((.Actions.UpdateUser .ID).OpenWhenHidden true).JS }}">defer</a>
```

→ `data-on-click="@patch('/users/42')"` and
`@patch('/users/42', {openWhenHidden: true})` (single quotes html-escaped to
`&#39;`, which the browser decodes back to `'`). `String()` remains for plain
attribute / text contexts and debugging.

### Conditional emission

- `DatastarEventTemplateData`: emitted when a route uses an `elements` arg (usage-gated,
  like `SSETemplateData` is gated on `UsesSSE`).
- `DatastarTemplateData` + `.Actions()` types: emitted when `--use-datastar` is set.
- Signal JSON helper + buffer pool: emitted when a route uses a `signal` arg.

## signal JSON marshaling + the --output-jsonv2 flag

By default the generated `datastarMarshalSignals(buf *bytes.Buffer, v any) error`
helper imports the standard library `encoding/json` and uses `json.Marshal` +
`buf.Write`. The `--output-jsonv2` flag (opt-in, off by default) switches it to
import `encoding/json/v2` and marshal via `json.MarshalWrite(buf, v)` directly
into the buffer the callback already obtained from the existing `bytesBufferPool`
(no separate pool is introduced).

**No GOEXPERIMENT auto-detection.** An earlier draft read `go env GOEXPERIMENT`
at generation time, but on a machine where jsonv2 is globally enabled that
emitted `encoding/json/v2` for *every* codebase — including ones not built with
the experiment — and the dropped-import bug then let goimports substitute the
`go-json-experiment` backport. The flag makes the default always `encoding/json`;
`go-json-experiment` is never referenced.

**Toolchain note:** `--output-jsonv2` requires the *consuming* module's go
directive to be `go 1.25`+ and a toolchain with `GOEXPERIMENT=jsonv2`. Because
`astgen.FormatFile` runs goimports, the import must be registered *before*
goimports runs (see the import-ordering note below), otherwise goimports cannot
resolve the bare `json.MarshalWrite`.

## Generated handler behavior

For a datastar route the generated handler:

- **Streaming (`elements` present):** set `Content-Type: text/event-stream`,
  obtain an `http.Flusher`, and wire the `elements`/`signal` callbacks so each
  call renders/marshals and flushes one event frame. Reuses the existing SSE
  handler scaffolding.
- **`signal` standalone:** set `Content-Type: application/json`, run the callback,
  write the marshaled bytes as the body.
- **`script` standalone:** set `Content-Type: text/javascript`, render the
  same-named template, write the body.
- **fragment:** unchanged from today (`text/html`).

## Testing

txtar reference tests under `cmd/muxt/testdata/`, following the existing
convention and asserting the SSE wire bytes via `go test` like `reference_sse.txt`:

- `reference_datastar_elements.txt` — patch-elements frame incl. `.Selector`/`.Mode`.
- `reference_datastar_signals.txt` — standalone `application/json` body.
- `reference_datastar_signals_stream.txt` — inline `datastar-patch-signals` event
  alongside an `elements` stream, incl. `onlyIfMissing`.
- `reference_datastar_script.txt` — `text/javascript` body.
- `reference_datastar_actions.txt` — `.Actions()` rendering `@verb(...)` with
  `OpenWhenHidden` via `.JS`, and `muxt check` type-checking
  `.Actions`/`.JS`/`.OpenWhenHidden`. (check coverage is folded into the
  reference fixtures rather than a separate `_check` fixture.)
- `err_use_htmx_and_datastar.txt` — mutual-exclusion error.
- `err_datastar_arg_without_flag.txt` — `elements`/`signal`/`script` used without
  `--use-datastar`.

## Resolved decisions & implementation notes (post-build)

1. **`script` mechanics — confirmed.** A `script` arg renders the same-named
   template into a `text/javascript` body (non-streaming). To emit a script
   mid-stream, append a `<script>` via `elements`.
2. **Stream determination — confirmed.** A route streams iff it declares an
   `elements` arg; a lone `signal` is `application/json`; a lone `script` is
   `text/javascript`. A signals-only *stream* (multiple `signal`s, no `elements`)
   is deferred.
3. **`sse` under datastar mode — revised: rejected, not an escape hatch.**
   Because `DatastarEventTemplateData` occupies the SSE type slot
   (`config.SSETemplateDataType`), a plain `sse` arg under `--use-datastar`
   errors with a message pointing at `elements`. Datastar's streaming is
   `elements`/`signal`; generic `sse` is a non-datastar feature.
4. **`onlyIfMissing` is honored only on streaming routes.** A standalone `signal`
   (`application/json` body) cannot carry the `onlyIfMissing` SSE data line, so
   it is ignored there; use a streaming (`elements`) route to emit it.
5. **`check` needs no changes (Section 6 superseded).** `check` resolves template
   data methods from the on-disk generated types, so it type-checks
   `.Selector`/`.Actions`/etc. automatically once generation emits them. The
   shared `--use-htmx`/`--use-datastar` flags land on `generate` only; `check`
   gets the shared flag with the future datastar *semantic* checks.
6. **Import ordering.** The generated import block is now assembled *after* all
   declarations (including the conditionally-appended Datastar signal helper) so
   the late `encoding/json/v2` import survives goimports.
7. **Actions in `data-on-*` need `template.JS`.** See the `.JS()` note above.
```