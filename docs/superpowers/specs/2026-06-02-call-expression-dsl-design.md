# The template-name call expression as an extensible DSL

**Status:** Direction (design) — phased; each phase gets its own spec → plan → PR.
**Date:** 2026-06-02

## Context

muxt template names end in a "call" parsed as a Go expression
(`internal/muxt/definition.go` `parseHandler` → `parser.ParseExprFrom` →
`*ast.CallExpr` whose `Fun` is an `*ast.Ident`). `checkArguments` already recurses
into nested `*ast.CallExpr` args, so `Method(ctx, Helper(form))` already works.
muxt also supports multiple template variables (variadic
`--use-templates-variable`, PR #68, merged).

Building the Datastar feature surfaced rough edges that trace back to the call
grammar not being expressive enough:

1. **No request-side input binding** — no way to bind a JSON body to a struct.
2. **Response shape and frontend coupling encoded by magic reserved *argument*
   names** (`sse`/`elements`/`signal`/`script` callbacks) and by *mutating*
   `TemplateData` under `--use-htmx`/`--use-datastar`.
3. **`script` through html/template** HTML-escapes its JS body.

Idea: the call is a Go expression, so muxt recognizes a small set of
**pseudo-functions** in three roles — a **framing wrapper** (frontend), a
**representation wrapper** (transport), and **argument wrappers** (input binding).

## The model

A route call composes as `frame( represent( Method(args…) ) )`, where each layer
is optional:

```
datastar( sse( Clock(ctx, send) ) )                            // datastar patch-elements stream
datastar( marshalJSON( Increment(ctx, unmarshalJSON(body)) ) ) // read + write signals JSON
htmx( Index(ctx) )                                             // html render w/ HTMXTemplateData
Index(ctx)                                                     // plain html render w/ TemplateData
```

### Framing wrapper (outermost): `htmx(...)` / `datastar(...)`

Selects the **template-data type** the handler renders with and, for SSE, the
**event marshaler**:

- `datastar(...)` → `DatastarTemplateData` (with `.Actions()` etc.) and the
  Datastar `patch-elements`/`patch-signals` event marshalers
  (`DatastarEventTemplateData.WriteTo`).
- `htmx(...)` → a new **`HTMXTemplateData`** carrying the HX* helpers and the
  generic `data:`-line event marshaler. This **splits the HX* helpers out of
  `TemplateData`** (today `--use-htmx` mutates `TemplateData`) into a dedicated
  type, parallel to `DatastarTemplateData`.

`--use-htmx` / `--use-datastar` **wrap every route's call** in `htmx(...)` /
`datastar(...)` — there is **no per-route opt-out** under the flag. To mix
frontends or leave some routes unwrapped, **omit the flag and write the wrappers
explicitly per route**. Framing is per-frontend and pluggable (the `WriteTo`
pattern that already exists); HTMX/Fixi use the generic `data:` marshaler,
Datastar its patch-* marshalers — not generalized into one renderer because the
wire prefixes genuinely differ.

### Representation wrapper: `sse(...)` / `marshalJSON(...)` / none

- **none** → traditional `templates.ExecuteTemplate`, `text/html`.
- **`sse(Call(...))`** → Server-Sent Events. Inside, the handler is either:
  - a **channel or iterator** return — `<-chan T`, `iter.Seq[T]`, or
    `iter.Seq2[T, error]`; each value is one event rendered via the route's
    `define` body. An `iter.Seq2` error **replaces the entries on the event
    template-data error list**; channels carry values only (no error form — send a
    struct with an error field and read it via `.Result` if needed); or
  - an **error/nothing** return that takes a **`send`** callback (renders the
    `define` body and sends one event). Optional **`sendIdentifier`** callbacks
    render same-named templates (`sendClock` → template `Clock`); bare `send` →
    the `define` body. This `send`/`sendX` family replaces the `sse`-prefix
    convention and enables reuse via `{{template "Clock"}}`. A send callback may
    itself be **wrapped**: `marshalJSON(sendStatus)` makes it marshal its value as
    JSON (a `patch-signals` frame under `datastar`) instead of rendering a template
    (a `patch-elements` frame), so one stream can carry multiple element *and*
    signal senders. `execute` is **unaffected** — it remains the callback for
    non-SSE handlers that control when the template renders. Wrapper and callbacks
    coexist on one route.
- **`marshalJSON(Call(...))`** → marshal the result as `application/json`
  (`encoding/json`, or `encoding/json/v2` under `--output-jsonv2`). Output dual of
  `unmarshalJSON`. Future `marshalForm`/`marshalXML` possible.

### Argument wrappers: input binding

A reserved **`body`** identifier (`io.Reader`) plus wrappers decoding it into the
**Go type of the method parameter at that position**:

- **`unmarshalJSON(body)`** — `io.ReadAll`+`json.Unmarshal`; under
  `--output-jsonv2`, `encoding/json/v2` `UnmarshalRead(body, &v)`.
- **`unmarshalForm(body)`** — the existing **`form` arg on non-GET methods is
  sugar** for this (on GET, `form` binds the query — sugar holds for non-GET only).
- **`signals`** (datastar sugar) ≡ `unmarshalJSON(body)`.
- Future: `unmarshalXML` (probably never).

## Phases (each its own spec → plan → PR)

**Phase 0 — record the decisions as ADRs** in `docs/explanation/decisions/`
(`00006`–`00010`).

**Phase 1 — input binding.** `body` (`io.Reader`); `unmarshalJSON` (v1+v2);
`unmarshalForm` (recast `form`); `signals` sugar. Smallest, highest-value; reuses
the nested-`*ast.CallExpr` arg machinery and `createMethodSignature` parameter
inference.

**Phase 2 — representation wrappers.** `sse(...)` (channel/iterator or
`send`/`sendX`) and `marshalJSON(...)`. Extend the SSE handler scaffold so the
`define` body is rendered by the returned iterator/channel or bare `send`, and
`sendX` callbacks render same-named templates.

**Phase 3 — framing wrappers + `HTMXTemplateData` split.**
`htmx(...)`/`datastar(...)` select the template-data type and event marshaler;
`--use-htmx`/`--use-datastar` auto-wrap every call. Move HX* helpers off
`TemplateData` into `HTMXTemplateData`. This subsumes the dev-only
`elements`/`signal`/`script` arg families (patch-elements = `datastar(sse(…))`,
patch-signals = `datastar(marshalJSON(…))`).

**Later — `script` (text/javascript) ergonomics (blocked).** Needs a template var
parsed with `text/template`. Variadic template vars exist (PR #68); the missing
piece is letting a variable use a **different template package** so a `script`
route renders from a `text/template` var. Until then the `json`→`template.HTML`
helper is the workaround.

**Later — check-side value-add (deferred).** `.Actions.X.Path(...any)` constructor
with `muxt check` validating args against the route's path-parameter types;
`signal("name")` helpers checked against attributes / SSE patch-signals bodies.
Where `muxt check` stops being a thin wrapper over `github.com/typelate/check`.

## Compatibility

- The SSE response forms (`sse`/`sseX` args) and datastar arg families
  (`elements`/`signal`/`script`) shipped only in dev pre-releases
  (`v0.20.0-dev.N`), not GA — replace them cleanly, no deprecation/dual support.
- `--use-htmx` currently mutates `TemplateData`; Phase 3 moves to the `htmx(...)`
  wrapper + `HTMXTemplateData`.
- GA input args stay, recast as sugar (`form` non-GET ≡ `unmarshalForm(body)`).
  Phase 1 only adds optional syntax.

## Settled decisions

- **`send`** is the SSE event callback (uniform with `sendX`); **`execute`** is
  retained for non-SSE handlers.
- **Per-frame framing**: bare `sendX` renders a template → `patch-elements`;
  `marshalJSON(sendX)` marshals → `patch-signals`.
- **Frontend mixing**: omit the flag and wrap explicitly per route; the flag has
  no per-route opt-out.
- **Channel errors**: not required — `iter.Seq2[T, error]` is the error-aware
  iterator form; channels carry values only.

## Verification (per phase, when implemented)

- Phase 1: txtar fixtures binding a JSON body to a struct (v1 + `--output-jsonv2`);
  `muxt check` type-checks the bound struct; a datastar example reading posted
  signals server-side.
- Phase 2: fixtures for `sse(iter.Seq)`, `iter.Seq2` error replacement, channel
  sources, `send`/`sendX`, `marshalJSON`; assertions on the SSE/JSON wire output.
- Phase 3: fixtures for `htmx(...)`/`datastar(...)` selecting the template-data
  type and marshaler; `--use-htmx`/`--use-datastar` auto-wrap; `HTMXTemplateData`
  has the HX* helpers and `TemplateData` no longer does.
