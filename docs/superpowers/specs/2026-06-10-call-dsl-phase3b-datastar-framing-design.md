# Phase 3b — datastar framing wrapper (and removal of the elements/signal/script arg families)

> Design spec. On approval → `writing-plans` produces the implementation plan.
> Combines what the direction doc called Phase 3b (datastar framing) and Phase 3c
> (remove the dev-only arg families) into one migration: the old args are dev-only
> (shipped in `v0.20.0-dev.N`, never GA), so there is no deprecation window — the
> wrapper replaces them and the arg machinery is deleted in the same branch.

## Goal

Finish the framing layer of the call-expression DSL. Phase 3a introduced the
outer **framing wrapper** mechanism with `htmx(...)`. Phase 3b adds the
`datastar(...)` framing wrapper and retires the dev-only datastar **argument**
families (`elements` / `signal` / `script`). After this phase, datastar codegen is
driven entirely by **framing + representation** (`datastar(sse(...))`,
`datastar(marshalJSON(...))`, `datastar(Method(...))`), not by reserved argument
names.

## Background

A route call composes as `frame( represent( Method(args…) ) )`. Phase 1 added
argument wrappers (`unmarshalJSON(body)`), Phase 2 added representation wrappers
(`sse(...)`, `marshalJSON(...)`) with the `send`/`sendX` event-callback model, and
Phase 3a added the `htmx(...)` framing wrapper plus a per-framing template-data
type (`HTMXTemplateData`) and a `--use-htmx` auto-wrap. The Phase 3a hooks reused
here are: the `Framing` enum + `FramingWrapperHTMX` const + parse-time strip in
`parseHandler` (`internal/muxt/definition.go`), and `effectiveFraming` /
`renderTemplateDataType` + per-framing type emission (`internal/generate/routes.go`).

Today datastar is gated on `--use-datastar` (`config.Datastar`) **plus** three
reserved render-callback arguments recognized in `definition.go`
(`IsElementsArgument` / `IsSignalArgument` / `IsScriptArgument`, surfaced as
`def.UsesElements/Signal/Script/Datastar`). The handler dispatch in `routes.go`
(`if config.Datastar && def.UsesDatastar()`) routes to `datastarMethodHandlerFunc`
in `datastar_handlers.go`, which branches by argument shape into streaming
patch-elements, non-streaming signal JSON, and non-streaming script JS handlers.
This whole arg-driven path is replaced.

## Datastar protocol facts (verified against data-star.dev/reference/actions)

Datastar backend actions (`@get`/`@post`/`@put`/`@patch`/`@delete`) accept four
response content types, each with optional response headers:

- **`text/event-stream`** — Datastar SSE events (`datastar-patch-elements`,
  `datastar-patch-signals`).
- **`text/html`** — elements patched into the DOM; controlled by response headers
  **`datastar-selector`**, **`datastar-mode`** (`outer`|`inner`|`remove`|`replace`|
  `prepend`|`append`|`before`|`after`, default `outer`),
  **`datastar-use-view-transition`**.
- **`application/json`** — JSON object merged into signals automatically; response
  header **`datastar-only-if-missing`** (`true` → only patch signals not already
  present).
- **`text/javascript`** — executed in the browser.

(Header names above are verbatim and lowercase.)

## Design

### 1. Parser — recognize and strip `datastar(...)`

`internal/muxt/definition.go`:

- Extend the `Framing` enum with `FramingDatastar`; add
  `const FramingWrapperDatastar = "datastar"`.
- In `parseHandler`, the existing outer-framing strip block (which today only
  recognizes `htmx`) recognizes `datastar` as well, recording the framing and
  unwrapping its single call argument before the representation (`sse`/
  `marshalJSON`) unwrap runs. At most one framing wrapper per call (bad arity and
  "must be a call" errors mirror htmx). Composition: `datastar(sse(...))`,
  `datastar(marshalJSON(...))`, `datastar(Method(ctx))`.

### 2. `--use-datastar` auto-wrap (mirror htmx, ADR 7)

`effectiveFraming(config, def)`:

```
if def.Framing() != FramingNone { return def.Framing() }   // explicit wins
if config.HTMXHelpers          { return FramingHTMX }
if config.Datastar             { return FramingDatastar }
return FramingNone
```

No per-route opt-out under `--use-datastar`; mix frontends by omitting the flag
and writing `datastar(...)` / `htmx(...)` explicitly. `--use-htmx` /
`--use-datastar` remain mutually exclusive (unchanged check).

### 3. Three datastar template-data types (per-framing emission)

Emitted only when datastar framing is used in the file (the per-framing emission
introduced in 3a is extended with a datastar branch). Each is the base
template-data shape plus a datastar-specific surface:

- **`DatastarTemplateData[R,T]`** — render type for non-SSE, non-JSON datastar
  routes (`datastar(Method(ctx))`, `text/html`). Surface: `.Actions()` (backend
  action expressions, §5) **plus** non-streaming HTML patch-header setters
  `.Selector(string)`, `.Mode(string)`, `.UseViewTransition(bool)` that set
  `datastar-selector` / `datastar-mode` / `datastar-use-view-transition` response
  headers. Plus the base helpers (`.Result`, `.Request`, `.StatusCode`, `.Header`,
  `.Redirect`, …).
- **`DatastarEventTemplateData[R,T]`** — patch-elements **stream frame** type, the
  SSE event type when framing = datastar and representation = sse. Surface
  (unchanged from today): `.Selector`, `.Mode`, `.Namespace`, `.UseViewTransition`
  emitted as SSE `data:` lines; `WriteTo` emits `event: datastar-patch-elements`.
- **`DatastarSignalsTemplateData[R,T]`** — non-streaming patch-signals type, for
  `datastar(marshalJSON(Method(...)))`. The response **body** is the JSON encoding
  of the method result (`Content-Type: application/json`); Datastar merges it into
  signals. The route's `define` body **is still evaluated, its output discarded**,
  so it may set the single allowed header `datastar-only-if-missing: true` via its
  one setter `.OnlyIfMissing()` (no other settable surface; base accessors like
  `.Result`/`.Request`/`.Receiver` are available for conditions). `muxt check`
  type-checks these templates against `DatastarSignalsTemplateData`.

### 4. Representation × datastar framing

- **`datastar(sse(Method(ctx, send[, sendX…][, marshalJSON(sendSig)…])))`** →
  `text/event-stream` patch-elements stream. `send`/`sendX` render templates as
  `datastar-patch-elements` frames via `DatastarEventTemplateData`. A
  `marshalJSON(sendX)` send inside the stream emits inline `datastar-patch-signals`
  frames. Per the Phase 2 model the SSE transport (event-stream headers, flush,
  iterator/`send` mechanics) is generic; only the marshaler differs by framing.
  **Streaming signal `onlyIfMissing` is kept per-frame:** under datastar a
  `marshalJSON(sendX)` callback is `func(T, bool) error`, the bool setting
  `only-if-missing` on that patch-signals frame (this is the one place a
  `marshalJSON(sendX)` callback shape differs between generic SSE — `func(T) error`
  — and datastar framing).
- **`datastar(marshalJSON(Method(...)))`** → §3 `DatastarSignalsTemplateData`
  (`application/json` body + optional `datastar-only-if-missing` header).
- **`datastar(Method(ctx))`** → `text/html` render with `DatastarTemplateData`.

### 5. `.Actions()`

`.Actions()` (and the `DatastarActions` / `DatastarAction` builder types in
`datastar_actions.go`) are emitted on the datastar render type
(`DatastarTemplateData`) and only when datastar framing is used in the file —
moved from today's unconditional emission in datastar mode into the per-framing
emission path. Behavior of the action builder itself is unchanged.

### 6. Remove the arg families (the 3c half)

Delete from `internal/muxt/definition.go`: `IsElementsArgument`,
`IsSignalArgument`, `IsScriptArgument`, `IsDatastarArgument`, the
`UsesElements/Signal/Script/Datastar` accessors, the `elements`/`signal`/`script`
scope-identifier constants, and their handling in `checkArguments`. Delete the
arg-driven dispatch and shape handlers in `internal/generate/datastar_handlers.go`
(the `config.Datastar && def.UsesDatastar()` branch and the
elements/signal/script handler builders). Datastar streaming/signals/render is
now selected by framing + representation only.

**`script` is dropped** with no replacement — proper `text/javascript` ergonomics
remain blocked on a `text/template`-typed template variable (direction doc
"Later"). The existing `json` → `template.HTML` workaround stays available for
embedding JS data in HTML.

The `DatastarEventTemplateData` marshaler and `datastarMarshalSignals` helper are
**retained** (now driven by the wrapper path). The streaming patch-elements +
inline patch-signals frame emitters are reused.

### 7. CLI

Add, mirroring `--output-htmx-template-data-type` (3a):

- `--output-datastar-template-data-type` (default `DatastarTemplateData`)
- `--output-datastar-event-template-data-type` (default `DatastarEventTemplateData`)
- `--output-datastar-signals-template-data-type` (default `DatastarSignalsTemplateData`)

Each validated as a Go identifier and round-tripped through `configToArgs` (the
two consistency fixes applied to the htmx flag). The existing datastar default
names (currently driven through `--output-template-data-type` /
`--output-sse-template-data-type` under `--use-datastar`) move to these dedicated
flags so the framing model has one render-type flag per framing.

## Components and boundaries

- `internal/muxt/definition.go` — framing recognition only (parser). One added
  enum value, one const, the strip-block extension; the arg-family deletions.
- `internal/generate/datastar_event_template_data.go` — `DatastarEventTemplateData`
  (retained; patch-elements stream marshaler).
- `internal/generate/datastar_signals.go` — `datastarMarshalSignals` (retained) +
  new `DatastarSignalsTemplateData` type with `.OnlyIfMissing()`.
- `internal/generate/datastar_actions.go` — `.Actions()` + builders (retained;
  accessor moved to per-framing emission). (The pre-existing
  `datastarActionBuilderSource` string-const → go/ast refactor remains a separate
  deferred task; not in scope here.)
- `internal/generate/datastar_template_data.go` (new or folded into
  `template_data.go`) — `DatastarTemplateData` (render type: `.Actions()` + HTML
  patch-header setters + base).
- `internal/generate/routes.go` — `effectiveFraming` datastar branch,
  `renderTemplateDataType` datastar branch, per-framing emission datastar branch,
  representation×framing dispatch in `methodHandlerFunc` (datastar marshaler
  selection on the sse/marshalJSON paths).
- `internal/cli/commands.go` — three flags + defaults + validation + round-trip.

## Error handling / validation

- Bad framing arity / non-call argument: same errors as htmx (Phase 3a).
- `--use-htmx` + `--use-datastar`: mutually exclusive (unchanged).
- `datastar(marshalJSON(...))` templates type-check against
  `DatastarSignalsTemplateData` (only `.OnlyIfMissing` settable) via `muxt check`.
- After arg-family removal, a route using a bare `elements`/`signal`/`script`
  argument is just an ordinary argument/path value again (no longer reserved) —
  matching `reference_datastar_args_only_reserved_with_flag.txt`'s premise, now the
  default everywhere.

## Testing (txtar, behavioral — no grep assertions over generated source)

Per project preference, fixtures assert behavior: `muxt check` for static
type-validation of template method calls, and `exec go test` driving
`TemplateRoutes` over `httptest` to assert response status, `Content-Type`,
`datastar-*` headers, SSE `event:`/`data:` frame bytes, and JSON bodies.

- `reference_datastar_framing_elements.txt` — `datastar(sse(Method(ctx, send)))`
  streams `event: datastar-patch-elements` frames; `.Selector`/`.Mode` set the
  data lines.
- `reference_datastar_framing_signals_stream.txt` — inline `marshalJSON(sendSig)`
  in the stream emits `datastar-patch-signals` frames; per-frame `onlyIfMissing`
  bool honored.
- `reference_datastar_framing_signals.txt` — `datastar(marshalJSON(Method(...)))`
  returns `application/json` body; the template sets `datastar-only-if-missing` via
  `.OnlyIfMissing`; assert header + body.
- `reference_datastar_framing_render.txt` — `datastar(Method(ctx))` renders
  `text/html`; `.Selector`/`.Mode`/`.UseViewTransition` set the `datastar-*`
  response headers; `.Actions()` renders a backend-action expression.
- `reference_datastar_auto_wrap.txt` — `--use-datastar` auto-wraps a plain route
  to datastar framing (`.Actions()` available without an explicit wrapper).
- `reference_datastar_signals_jsonv2.txt` — migrate (jsonv2 marshaling under the
  wrapper).
- Migrate/replace every existing `reference_datastar_*.txt` to wrapper syntax;
  delete `reference_datastar_script.txt` (script removed) and add an `err_*`
  fixture asserting a former `script` arg is now an ordinary unknown argument /
  no longer special.
- `muxt check` negative: a `datastar(marshalJSON(...))` template calling a
  non-existent setter fails check.

## Out of scope

- `script` / `text/javascript` ergonomics (blocked on a `text/template` template
  variable).
- Non-datastar use of the `datastar-*` HTML patch headers.
- The `datastarActionBuilderSource` string-const → go/ast builder refactor
  (separate deferred task).
- `.Actions` check-side validation of path-parameter argument types (direction doc
  "Later — check-side value-add").

## Compatibility

The `elements`/`signal`/`script` arg families and the `--use-datastar` arg-driven
behavior shipped only in dev pre-releases — replaced cleanly, no deprecation or
dual support. `--use-datastar` continues to exist; it now auto-wraps routes in
`datastar(...)` instead of arming reserved argument names.