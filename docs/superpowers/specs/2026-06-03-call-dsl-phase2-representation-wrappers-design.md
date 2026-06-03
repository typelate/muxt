# Phase 2 — representation wrappers (`sse()` + `marshalJSON()`)

**Status:** Spec for review
**Date:** 2026-06-03
**Parent:** `docs/superpowers/specs/2026-06-02-call-expression-dsl-design.md`
(ADRs `docs/explanation/decisions/00006`, `00008`, `00009`).

## Context

This is Phase 2 of the call-expression DSL: add the response-side **representation
wrappers**. Today an SSE endpoint is flagged by a reserved `sse` callback argument,
extra streams use an `sse`-prefix naming convention (`sseClock` → template
`sseClock`), and there is no JSON-response path. Those `sse`/`sseX` arg forms
shipped only in dev pre-releases (`v0.20.0-dev.N`), not GA, so they are replaced
cleanly with no deprecation window.

The call is already a Go expression and the parser already recurses into nested
`*ast.CallExpr` arguments (Phase 1). Phase 2 recognizes two pseudo-functions at the
**outermost** call position instead — a representation wrapper around the method
call: `sse(Method(...))` and `marshalJSON(Method(...))`. An unwrapped call remains
an ordinary `text/html` `ExecuteTemplate` handler.

## Scope

**In:**
- `sse(Method(...))` representation wrapper, replacing the `sse`/`sseX` arg family.
- `send` / `sendX` SSE event callbacks (`send` → the route's `define` body;
  `sendClock` → template `Clock`, prefix stripped, remainder verbatim).
- Iterator and channel return shapes inside `sse(...)`: `<-chan T`, `iter.Seq[T]`,
  `iter.Seq2[T, error]`.
- `marshalJSON(sendX)` — wrap a send callback to marshal its value as JSON instead
  of rendering a template (a plain SSE `data:` JSON event in Phase 2).
- `marshalJSON(Method(...))` — standalone `application/json` response wrapper
  (`encoding/json`, or `encoding/json/v2` `MarshalWrite` under `--output-jsonv2`).

**Out (later phases):** framing wrappers `htmx(...)` / `datastar(...)` and the
`HTMXTemplateData` split (Phase 3); datastar `patch-elements` / `patch-signals`
framing of `sse()` / `marshalJSON(sendX)` (Phase 3); `marshalForm` / `marshalXML`
(not planned). The existing datastar `elements` / `signal` / `script` arg families
stay untouched in Phase 2 — Phase 3 subsumes them.

## Design

### Approach

Reuse the existing SSE transport — `streamMethodHandlerFunc`, `sseClosure`,
`SSETemplateData.WriteTo`, the `bytesBufferPool`, the flusher check, and the
serializing mutex. Phase 2 changes only the **trigger** (reserved `sse` arg → the
`sse(...)` wrapper), the **callback naming** (`sse`/`sseX` → `send`/`sendX`, with
`sendX` mapping to template `X` verbatim), and **adds** the iterator/channel return
path plus the `marshalJSON` codegen. The SSE wire framing stays the generic
`data:`-line marshaler (`SSETemplateData.WriteTo`) — per ADR 9 framing is a
per-frontend marshaler selected later by the Phase 3 framing wrapper.

### Recognizing the wrappers (parser, `internal/muxt/definition.go`)

- Reserve `sse` and `marshalJSON` as **outer** wrapper function names. Add a
  `Representation()` accessor returning an enum (`none` | `sse` | `marshalJSON`)
  computed from the outermost `*ast.CallExpr`'s `Fun` identifier.
- The inner expression (the method call) is unwrapped for argument checking, so
  `checkArguments` validates the inner method's args exactly as today (path params,
  `ctx`, `body`, `unmarshalJSON`/`unmarshalForm`, plus the new `send`/`sendX`
  callbacks). `marshalJSON` is reserved in **two positions**: outer (response
  wrapper) and around a `sendX` callback inside `sse(...)`.
- Reserve `send` and `sendX` callback names (`isReservedOrPrefixed(name, "send")`),
  replacing `IsSSEArgument`. **Remove** `IsSSEArgument`, `sseArg`, and the
  `TemplateNameScopeIdentifierSSE` reserved `sse` arg. `send`/`sendX` are reserved
  only inside an `sse(...)` wrapper; a bare `send` argument outside `sse(...)` is an
  "unknown argument" error.

### `sse(Method(...))` — two sub-modes by return type

`methodHandlerFunc` dispatches on `Representation() == sse`. The handler is one of
two sub-modes, chosen by inspecting the inner method's result type
(`sig.Results()`):

**Callback mode** — method returns `error` or nothing and takes callbacks:
- `send func(T) error` or `func() error` → renders the route's `define` body
  (`def.Name()`), one event per call.
- `sendClock func(T) error` → renders template `Clock` (strip the `send` prefix;
  the remainder, e.g. `Clock`, is the template name verbatim).
- Each callback is generated from the existing `sseClosure` + `SSETemplateData`
  path; the `sendX` variant differs only in the template name passed to
  `ExecuteTemplate` (the stripped remainder rather than `def.Name()`).
- The callback value type `T` comes from the method's declared callback parameter
  (or the synthesized signature when the method is undefined, reusing the
  `createMethodSignature` machinery from Phase 1).

**Return mode** — method returns a channel or iterator (new return-type
inspection):
- `<-chan T` → `for v := range ch { renderEvent(v) }`; each value renders the
  `define` body as one event.
- `iter.Seq[T]` → range-over-func; each yielded value renders one event.
- `iter.Seq2[T, error]` → each `(v, err)`; a non-nil `err` **replaces the entries
  on the event template-data error list** (the `SSETemplateData` error slot), then
  the `define` body renders with that error in scope. Channels carry values only —
  no error form.

The two sub-modes are **mutually exclusive** per route. A route that both returns
an iterator/channel and declares a `send`/`sendX` callback is a compile-time error.

**Unwrapped iterator/channel:** a method whose return type is a channel or iterator
but whose call is **not** wrapped in `sse(...)` is a compile-time error directing
the author to wrap it in `sse(...)`. (An unwrapped call expects a renderable result
or `error`, not a stream.)

### `marshalJSON(sendX)` — wrap a send callback

Inside `sse(...)`, a send callback may itself be wrapped: `marshalJSON(sendStatus)`
makes the callback **marshal its argument value as JSON** and write those bytes as
the event data, instead of executing a template. The transport is unchanged (the
generic `data:`-line marshaler frames the JSON bytes); only the data *source*
differs (marshal vs `ExecuteTemplate`). In Phase 2 this is a plain SSE JSON event;
Phase 3's `datastar(...)` reframes it as a `patch-signals` frame. JSON marshaling
follows the `--output-jsonv2` switch (`encoding/json` default;
`encoding/json/v2` `MarshalWrite` under the flag).

### `marshalJSON(Method(...))` — standalone JSON response

`methodHandlerFunc` dispatches on `Representation() == marshalJSON`. The inner
method returns `(T)` or `(T, error)`:
- marshal `T` to the response with `Content-Type: application/json` and status
  `200`;
- `encoding/json` `json.Marshal` + `response.Write` by default;
  `encoding/json/v2` `json.MarshalWrite(response, v)` under `--output-jsonv2`
  (reuse the import-management pattern in `internal/generate/datastar_signals.go`);
- a non-nil method error, or a marshal error, responds `500` (reuse the existing
  500 error path). A method with **no** return value under `marshalJSON` is a
  compile-time error (nothing to marshal).

Generated into a new file `internal/generate/marshal_json.go`, mirroring the
`datastar_signals.go` JSONV2 branching. `marshalJSON` is the output dual of
Phase 1's `unmarshalJSON`.

### `--use-datastar` interaction & `execute`

`sse()` is frontend-agnostic in Phase 2 — it always emits generic SSE via
`SSETemplateData.WriteTo`, regardless of `--use-datastar`; Phase 3 layers the
datastar marshaler selection on top. The existing datastar `elements` / `signal` /
`script` arg families are unchanged. The old `config.Datastar && def.UsesSSE()`
guard is removed (the `sse` *arg* no longer exists). `execute` is unchanged — it
remains the non-SSE single-shot callback for handlers that control when their one
template renders.

### `muxt check`

Representation wrapping changes the receiver method signature and the response
content type, not the template-data type, so template type-checking is unaffected.
The generated receiver interface lists each method with its parameter and result
types (the declared types, or the synthesized callback/return types for undefined
methods), exactly as for other calls.

### Parser & generator touch points

- **Parser (`internal/muxt/definition.go`):** add the `sse` / `marshalJSON` outer
  wrapper recognition and `Representation()`; reserve `send`/`sendX`; remove the
  `sse` arg family (`IsSSEArgument`, `sseArg`, `TemplateNameScopeIdentifierSSE`);
  unwrap the outer wrapper before `checkArguments`.
- **Dispatch (`methodHandlerFunc`, `internal/generate/routes.go`):** branch on
  `Representation()` — `sse` → SSE handler (callback vs return sub-mode);
  `marshalJSON` → JSON response handler; `none` → existing `ExecuteTemplate`/
  `execute` path.
- **SSE codegen (`internal/generate/routes.go`):** retarget `sseMethodHandlerFunc`
  to the wrapper; `sendX` template-name mapping (strip `send` prefix); add
  return-type inspection and the channel / `iter.Seq` / `iter.Seq2` range codegen
  (import `iter`); `iter.Seq2` error → event error list.
- **JSON codegen (`internal/generate/marshal_json.go`, new):** the `marshalJSON`
  response handler and the `marshalJSON(sendX)` callback variant, reusing the
  `JSONV2` import branching.

## Edge cases

- `sse(...)` callback mode and return mode are mutually exclusive — error if both.
- An iterator/channel-returning method not wrapped in `sse(...)` — error.
- `marshalJSON(Method(...))` where the method returns no value — error.
- `send`/`sendX` used outside an `sse(...)` wrapper — "unknown argument" error.
- `*iter.Seq2[T, error]` and channels only carry the value through the generic
  marshaler; the `iter.Seq2` error path reuses the `SSETemplateData` error slot.
- `marshalJSON` JSONV2 paths only compile under `--output-jsonv2` (the v2 package),
  consistent with the rest of the v2 path.

## Testing (txtar fixtures in `cmd/muxt/testdata/`)

- `reference_sse_wrapper.txt` — `sse(Method(ctx, send))` renders the `define` body;
  assert event frames on the wire.
- `reference_sse_sendx.txt` — `sse(Method(ctx, send, sendClock))` renders the
  `define` body and template `Clock`; assert both event shapes.
- `reference_sse_iter_seq.txt` — `sse(Method(ctx))` returning `iter.Seq[T]`; assert
  one event per yielded value.
- `reference_sse_iter_seq2_error.txt` — `iter.Seq2[T, error]`; assert a yielded
  error reaches the event template-data error list.
- `reference_sse_chan.txt` — `<-chan T` source; assert one event per value.
- `reference_sse_marshal_send.txt` — `marshalJSON(sendStatus)` emits a JSON `data:`
  event (default `encoding/json`).
- `reference_marshal_json.txt` — `marshalJSON(Method(ctx))` →
  `Content-Type: application/json`, marshaled body (default `encoding/json`).
- `reference_marshal_json_jsonv2.txt` — `--output-jsonv2`, generation-only:
  asserts `json.MarshalWrite` and the `encoding/json/v2` import, never the backport.
- `err_sse_iter_and_send.txt` — both an iterator return and a `send` callback.
- `err_sse_unwrapped_iter.txt` — iterator return without `sse(...)`.
- `err_marshal_json_no_value.txt` — `marshalJSON` of a method with no return value.
- Migrate the existing `reference_sse_*` / `err_sse_*` fixtures from the `sse`/`sseX`
  arg forms to the `sse()` wrapper + `send`/`sendX`.
- `muxt check` passes in each reference fixture.

## Open questions

None outstanding; settled during design: combined single Phase 2 (not split into
sub-phases); `sse()` under `--use-datastar` emits generic frames in Phase 2 (datastar
framing is Phase 3); unwrapped iterator/channel returns are an error; `marshalJSON`
method/marshal errors respond `500`; the `sse`/`sseX` arg forms are removed cleanly.
