# Phase 1 — request input binding (`body` + `unmarshalX`)

**Status:** Spec for review
**Date:** 2026-06-02
**Parent:** `docs/superpowers/specs/2026-06-02-call-expression-dsl-design.md`
(ADRs `docs/explanation/decisions/00006`, `00010`).

## Context

This is Phase 1 of the call-expression DSL: add request-side input binding. Today
muxt's `form` arg parses `request.Form`, but there is no way to bind a JSON
request body to a struct — the gap the Datastar example hit (it had to force
`contentType:'form'`). The call is already a Go expression and `checkArguments`
already recurses into nested `*ast.CallExpr` args, so an argument-wrapper is a
natural fit.

## Scope

**In:** the `body` reserved identifier; the `unmarshalJSON(body)` and
`unmarshalForm(body)` argument wrappers; recasting `form` (non-GET) to share the
`unmarshalForm` codegen.

**Out (later phases):** `signals` datastar sugar (Phase 3, reserved under
`--use-datastar`); response wrappers `sse`/`marshalJSON` (Phase 2); framing
wrappers (Phase 3); `unmarshalXML` (not planned).

## Design

### `body` reserved identifier

Add `body` to `patternScope()` (`internal/muxt/definition.go`), typed `io.Reader`
in `defaultTemplateNameScope` (`internal/generate/routes.go`). The generated
handler binds `body := request.Body`. It is usable standalone
(`Method(ctx, body)` → an `io.Reader` parameter) and as the argument to the
`unmarshalX` wrappers. `body` is a single-use stream; a route that consumes it
twice (e.g. standalone `body` *and* `unmarshalJSON(body)`) is the author's
responsibility — documented, not enforced.

### `unmarshalJSON(body)` argument wrapper

A recognized wrapper pseudo-function (not a receiver method). The **decode target
type is the receiver method's parameter at that call position**:

- **Defined method, concrete param type `T`:**
  - default (`encoding/json`): `b, err := io.ReadAll(body)` then
    `json.Unmarshal(b, &v)` into a `T`;
  - `--output-jsonv2`: `encoding/json/v2` `json.UnmarshalRead(body, &v)` (streams
    from the reader, no `ReadAll`).
- **Undefined method (inferred signature) — raw pass-through, no error:** the
  synthesized parameter type is `json.RawMessage` (default) or `*jsontext.Decoder`
  (`encoding/json/jsontext`, under `--output-jsonv2`). Codegen for these target
  types passes the raw/streaming form to the handler rather than decoding:
  `json.RawMessage(b)` from `io.ReadAll(body)`; `jsontext.NewDecoder(body)`.
  A *defined* method may also declare `json.RawMessage` / `*jsontext.Decoder` to
  opt into the same pass-through — i.e. codegen branches on the **target type**,
  and the undefined default simply selects that raw target.

Decode failures respond **400** and accumulate into the template-data error list,
reusing the existing scalar-parse error path (`parseErrBlock` /
`templateDataParseErrBlock`, `http.StatusBadRequest`).

### `unmarshalForm(body)` argument wrapper + `form` recast

`unmarshalForm(body)` decodes the form-encoded body into the parameter type,
reusing the existing form binding (`callParseForm` + `appendStructFieldParseStatements`
for structs, `net/url.Values` for the map form, `name:"..."` tags, 400 on error).
A **non-GET `form` argument routes through the same codegen** so the two are
genuinely identical (true sugar). A **GET `form` keeps its query-string
behavior** (`request.Form` includes the query), so the equivalence is non-GET
only.

### Parser & generator touch points

- **Parser (`internal/muxt/definition.go`):** add `body` to the scope consts and
  `patternScope()`. In `checkArguments`, recognize `unmarshalJSON` / `unmarshalForm`
  as reserved wrapper names whose single argument must be `body` (so they are
  neither "unknown argument" nor treated as receiver-method calls). These wrapper
  names are **always reserved** (generic input binding, not flag-gated).
- **Type determination (`createMethodSignature`, `internal/generate/routes.go`):**
  for an `unmarshalJSON(body)` / `unmarshalForm(body)` arg, the parameter type
  comes from the defined method; when undefined, synthesize the raw target
  (`json.RawMessage` / `*jsontext.Decoder` for JSON; the existing
  `net/url.Values` for form).
- **Codegen (`appendParseArgumentStatements`, the `*ast.CallExpr` case):** branch
  on the wrapper name *before* the receiver-method lookup; emit the decode/bind
  statements and replace the arg with the bound local, mirroring how `form` and
  nested-call results are wired today.
- **jsonv2:** reuse the `config.JSONV2` branching pattern from
  `internal/generate/datastar_signals.go` (import management via `astgen.Call`,
  `encoding/json` vs `encoding/json/v2`; add `encoding/json/jsontext` for the
  decoder).

### `muxt check`

Input binding changes the **receiver method signature**, not the template-data
type, so `check` is unaffected for template type-checking. The generated receiver
interface lists the method with its parameter type (the declared type, or the
synthesized `json.RawMessage` / `*jsontext.Decoder` / `url.Values` for undefined
methods), exactly as for other args.

## Edge cases

- `unmarshalJSON`/`unmarshalForm` accept exactly one argument and it must be
  `body`; anything else is an error.
- Reusing `body` in more than one position double-reads the stream (second read is
  empty) — documented caveat.
- `*jsontext.Decoder` only compiles under `--output-jsonv2` (the jsontext package
  is part of the json v2 experiment), consistent with the rest of the v2 path.

## Testing (txtar fixtures in `cmd/muxt/testdata/`)

- `reference_unmarshal_json.txt` — `unmarshalJSON(body)` into a struct, asserting
  a posted JSON body binds and renders; run with the default `encoding/json`.
- `reference_unmarshal_json_jsonv2.txt` — generation-only (grep) asserting the
  `--output-jsonv2` path emits `json.UnmarshalRead` and the `encoding/json/v2`
  import, never the backport.
- `reference_unmarshal_json_undefined.txt` — undefined method binds
  `json.RawMessage` (default) and (`--output-jsonv2`, generation-only)
  `*jsontext.Decoder`.
- `reference_unmarshal_form.txt` — `unmarshalForm(body)` into a struct equals the
  `form` arg output; assert a non-GET `form` route generates identical binding.
- `err_unmarshal_json_bad_arg.txt` — `unmarshalJSON(x)` where `x` is not `body`.
- `muxt check` passes in each reference fixture.

## Open questions

None outstanding; settled during design: `form` non-GET shares the
`unmarshalForm(body)` codegen; `signals` deferred to the datastar phase; undefined
methods use `json.RawMessage` / `*jsontext.Decoder` pass-through (no error).
