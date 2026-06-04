# Phase 3a — framing wrappers: `htmx()` + the `HTMXTemplateData` split

**Status:** Spec for review
**Date:** 2026-06-03
**Parent:** `docs/superpowers/specs/2026-06-02-call-expression-dsl-design.md`
(ADRs `docs/explanation/decisions/00006`, `00007`, `00009`).

## Context

This is Phase 3a of the call-expression DSL: introduce the **framing wrapper** — the
outermost layer of the `frame( represent( Method(args) ) )` composition — and
implement the `htmx(...)` frontend. Today `--use-htmx` mutates the single
configurable template-data type by bolting on HX* helper methods; ADR 7 instead
makes the frontend a choice of **template-data type** (and, for SSE, event
marshaler), selected by an outer wrapper. Phase 3a delivers the framing mechanism
plus `htmx()`; the `datastar()` frontend (3b) and removal of the datastar
`elements`/`signal` arg families (3c) follow in their own cycles.

## Scope

**In:**
- A `Framing` enum (`FramingNone` | `FramingHTMX`) and `def.Framing()` accessor;
  parse-time recognition of `htmx(...)` as the outermost wrapper, stripped before
  the Phase-2 representation wrapper.
- A dedicated `HTMXTemplateData[R, T]` type: the full `TemplateData` base method
  set **plus** the HX* helpers. The HX* helpers are **removed** from `TemplateData`,
  which stays minimal.
- Generating the base template-data type **per distinct framing used in the file**
  (minimal `TemplateData` for unframed routes; `HTMXTemplateData` for htmx-framed
  routes). A route renders with the type matching its effective framing.
- `--use-htmx` auto-wraps every route in `htmx(...)` (effective framing = htmx) and
  defaults the htmx render-type name to `HTMXTemplateData`.

**Out (later phases):** `datastar(...)` framing + `FramingDatastar` and moving
`.Actions()` onto `DatastarTemplateData` (3b); removal of the datastar
`elements`/`signal` arg families (3c); the `script` arg family (blocked on per-var
template packages). The existing `--use-datastar` arg path is untouched in 3a.

## Design

### Framing recognition (parser, `internal/muxt/definition.go`)

Add to the parser, mirroring Phase 2's `Representation`:

```go
type Framing int

const (
	FramingNone Framing = iota
	FramingHTMX
)

const FramingWrapperHTMX = "htmx"
```

`def.Framing()` accessor + a `framing Framing` field on `Definition`. In
`parseHandler`, the wrapper layers are stripped **outermost-first**: detect
`htmx(...)` first (record `def.framing`, unwrap to the inner expression), then run
the existing Phase-2 representation detection on what remains, then `checkArguments`
on the innermost method call. So `htmx( sse( Method(args) ) )` yields
`framing = FramingHTMX`, `representation = RepresentationSSE`, and
`def.call`/`def.fun` pointing at `Method` — every existing consumer is unchanged.

`htmx(...)` takes exactly one argument (the inner call); `htmx` with the wrong
arity or a non-call argument is a parse error. `htmx` is reserved as an outer
framing name only; it is not a valid bare argument.

### `HTMXTemplateData` split (`internal/generate/template_data.go`)

Today `templateDataType()` generates one base type named `config.TemplateDataType`,
and `templateDataHTMXHelperMethods()` appends the HX* helpers to it when
`config.HTMXHelpers`. Phase 3a changes this to generate base types **per framing**:

- **`TemplateData`** (name `config.TemplateDataType`, default `TemplateData`):
  the base method set only (`Result`, `Ok`, `Err`, `Receiver`, `Request`,
  `StatusCode`, `Header`, `Redirect*`, `Path`, `String`, `MuxtVersion`). **No HX*
  helpers.** Emitted when the file has at least one `FramingNone` route.
- **`HTMXTemplateData`** (name `config.HTMXTemplateDataType`, default
  `HTMXTemplateData`): the same base method set **plus** the 19 HX* helpers
  (reusing `templateDataHTMXHelperMethods`, re-targeted to the `HTMXTemplateData`
  receiver). Emitted when the file has at least one `FramingHTMX` route.

The base-method generation is parameterized by the target type name and reused for
both. The HX*-helper generation is re-targeted from `TemplateData` to
`HTMXTemplateData`. Under `--use-htmx` every route is htmx-framed, so only
`HTMXTemplateData` is emitted; an unflagged file mixing `htmx(...)` and unframed
routes emits both.

A new config field `HTMXTemplateDataType string` (CLI `--output-htmx-template-data-type`,
default `HTMXTemplateData`) names the htmx type, parallel to `TemplateDataType`.

### `htmx()` selects the render type (`internal/generate/routes.go`)

`methodHandlerFunc` computes the **effective framing**: `def.Framing()` if set, else
`FramingHTMX` when `config.HTMXHelpers` (the `--use-htmx` flag), else `FramingNone`.
The non-SSE render path uses the template-data type for the effective framing
(`HTMXTemplateData` for htmx, `config.TemplateDataType` otherwise) wherever it
currently hard-codes `config.TemplateDataType` (the `td := TemplateData[...]{...}`
composite, the `.Path()`/error-accumulation helpers, etc.). Introduce a helper
`renderTemplateDataType(config, framing) string` and thread the effective framing
through the non-SSE handler builders.

**SSE is unaffected by htmx framing in 3a.** `htmx(sse(...))` uses the generic
`SSETemplateData` marshaler exactly as `sse(...)` does — HTMX SSE events are plain
`data:` frames and the HX* response-header helpers are not meaningful mid-stream.
htmx framing only swaps the non-SSE render type. (Datastar SSE framing is 3b.)

### `--use-htmx` auto-wrap

`--use-htmx` makes every route's effective framing `htmx` (ADR 7: no per-route
opt-out under the flag). Concretely, the effective-framing computation treats a
`FramingNone` route as `FramingHTMX` when the flag is set. A route with an explicit
`htmx(...)` wrapper under the flag is redundant but valid; an explicit
**conflicting** framing wrapper (a different frontend, once 3b adds `datastar()`)
under the flag is a generation error — for 3a there is no conflicting frontend yet,
so this is a forward-looking guard noted but not exercised.

`--use-htmx` and `--use-datastar` remain mutually exclusive (existing check). The
htmx render-type name defaults to `HTMXTemplateData`; `--output-template-data-type`
still names the minimal base type.

### Backward compatibility

Existing `--use-htmx` users are behaviorally unaffected: templates still call
`.HXRedirect` etc.; the route auto-wraps to htmx framing and renders with
`HTMXTemplateData`, which carries those helpers. The change is internal — the HX*
helpers move from `TemplateData` to a dedicated parallel type. `.Actions()` and the
datastar arg path are untouched in 3a.

### Parser & generator touch points

- **Parser (`internal/muxt/definition.go`):** `Framing` enum + `FramingWrapperHTMX`
  + `framing` field + `Framing()` accessor; strip `htmx(...)` first in
  `parseHandler`; reserve `htmx` as an outer-only name (not a bare arg).
- **Template-data (`internal/generate/template_data.go`):** parameterize base-type
  generation by target name; generate `TemplateData` (minimal) and/or
  `HTMXTemplateData` (base + HX*) per the framings used; remove the
  `config.HTMXHelpers`→`TemplateData` HX* attachment.
- **Dispatch/render (`internal/generate/routes.go`):** effective-framing
  computation; `renderTemplateDataType(config, framing)` helper; thread it through
  the non-SSE handler builders where the render type is chosen; emit the right base
  types based on the file's framings.
- **CLI (`internal/cli/commands.go`):** add `--output-htmx-template-data-type`
  (default `HTMXTemplateData`); `applyDefaults` sets the htmx type default. Keep
  `--use-htmx`/`--use-datastar` mutual exclusion.

### `muxt check`

Framing changes the render template-data type (which methods are available, e.g.
`.HXRedirect` only under htmx), not the route set. `check` resolves template-data
methods from the on-disk generated types via `go/types`, so it is mode-aware
automatically: an htmx-framed route exposes the HX* methods to its templates; an
unframed route does not.

## Edge cases

- `htmx(...)` accepts exactly one argument (the inner call); other arities/forms
  are parse errors.
- A bare `htmx` identifier as a method argument is an unknown argument (it is an
  outer framing name, not an arg).
- An unframed route's template referencing `.HXRedirect` now fails `muxt check`
  (the helper is no longer on minimal `TemplateData`) — the intended, documented
  consequence of the split.
- A file mixing `htmx(...)` and unframed routes (no flag) emits both
  `TemplateData` and `HTMXTemplateData`.
- `--use-htmx` + an explicit `htmx(...)` on a route: redundant, allowed.

## Testing (txtar fixtures in `cmd/muxt/testdata/`)

- `reference_htmx_framing.txt` — explicit `htmx(Index(ctx))` (no flag): renders
  with `HTMXTemplateData`; a template using `.HXRedirect` works; assert the HX
  response header is set on the wire.
- `reference_htmx_mixed.txt` — one `htmx(...)` route and one unframed route in a
  no-flag file: assert both `HTMXTemplateData` and `TemplateData` are generated and
  each route renders with the right one.
- `reference_htmx_template_data_minimal.txt` — an unframed route whose template
  calls `.HXRedirect` → `muxt check` fails (HX* no longer on `TemplateData`); assert
  the check error.
- `reference_htmx_auto_wrap.txt` — `--use-htmx` flag: a plain `Index(ctx)` route
  auto-wraps to htmx framing and renders with `HTMXTemplateData`; `.HXRedirect`
  works (migrated from / superseding the structure of `reference_htmx_helpers.txt`).
- Migrate `reference_htmx_helpers.txt` — its HX*-method grep assertions now target
  the `HTMXTemplateData` receiver; behavior/wire assertions unchanged.
- `err_htmx_bad_arity.txt` — `htmx()` with zero or two arguments → parse error.
- `muxt check` passes in each reference fixture (except the intentional
  minimal-data check-failure fixture, which asserts the failure).

## Open questions

None outstanding; settled during design (Approach A): generate base types per
framing used; htmx render-type name defaults to `HTMXTemplateData`; `htmx(sse(...))`
keeps the generic SSE marshaler; `--use-htmx` auto-wraps with no per-route opt-out;
3a is htmx-only with the `--use-datastar` arg path untouched.
