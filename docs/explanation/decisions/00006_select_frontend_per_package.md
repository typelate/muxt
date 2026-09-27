# 6 - Select the Frontend Library Per Package

## Context

To support htmx and Datastar I designed framing wrappers for the template name syntax: `htmx(Method(...))` and `datastar(Method(...))` would select a frontend-specific template data type per route, `--output-htmx` and `--output-datastar` would wrap every unframed route, and the two could mix in one package.

Prototyping surfaced incidental complexity:

- wrapper arity errors
- auto-wrap versus explicit-wrap interactions
- conditional emission of the base `TemplateData` when every route is framed
- a breaking move of the HX* helpers off the shared type

The wrapper also repeats a decision the template body already makes: `hx-*` or `data-*` attributes commit the file to a library, and a wrapper in the name can drift from the body.

A project serving both libraries already works with one package per frontend, each with its own template set and `muxt generate` invocation, registering routes on a shared `http.ServeMux`.

## Decision

Select the frontend library per package with generate flags (`--output-htmx`, later `--output-datastar`). `--output-htmx` and `--output-datastar` are mutually exclusive. Do not add framing wrappers to the template name syntax.

The flags extend the existing generated types ([CLI](../../reference/cli.md)) instead of adding library-specific ones.

## Status

Decided

## Consequences

- The template name grammar stays a single call expression; representation wrappers ([`sse`](../../reference/call-parameters.md#server-sent-events), [`marshalJSON`](../../reference/call-results.md#json-responses)) are unaffected.
- No type split or migration is needed for the HX* helpers.
- Datastar support is package-level configuration, not per-route state. Rendered (non-SSE) routes are unchanged.
- `muxt check` reports an `HX*` call in a package generated without `--output-htmx` only as a missing method.
