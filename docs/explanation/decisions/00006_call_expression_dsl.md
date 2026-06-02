# 6 - Extend the Call Expression into a Small DSL

## Context

A template name ends in a "call" that muxt parses as a Go expression
(`parseHandler` → `parser.ParseExprFrom` → an `*ast.CallExpr` whose `Fun` is an
identifier). `checkArguments` already recurses into nested call arguments, so
`Method(ctx, Helper(form))` works today.

Building the Datastar feature showed the grammar is not expressive enough:
request bodies cannot be bound to a struct, response shape and frontend are
encoded by magic reserved *argument* names (`sse`/`elements`/`signal`/`script`)
and by mutating `TemplateData`, and `script` output is HTML-escaped.

## Decision

Treat the call as a small DSL of recognized **pseudo-functions** in three roles,
composing as `frame( represent( Method(args…) ) )`:

- **framing wrappers** — `htmx(...)` / `datastar(...)` (the frontend);
- **representation wrappers** — `sse(...)` / `marshalJSON(...)` / none (transport);
- **argument wrappers** — `unmarshalJSON(body)` / `unmarshalForm(body)` (input).

Each role is its own decision record and implementation phase (see
[7](00007_framing_wrappers.md), [8](00008_sse_wrapper_and_send_callbacks.md),
[9](00009_event_framing_is_a_custom_marshaler.md),
[10](00010_request_input_binding.md)).

## Status

Decided

## Consequences

The grammar becomes more expressive without leaving Go-expression syntax. Magic
reserved-argument conventions are replaced by composable wrappers. Work is phased;
the full direction lives in
`docs/superpowers/specs/2026-06-02-call-expression-dsl-design.md`.
