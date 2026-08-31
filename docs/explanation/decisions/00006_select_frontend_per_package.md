# 6 - Select the Frontend Library Per Package

## Context

To support htmx and Datastar I designed framing wrappers for the template name
syntax: `htmx(Method(...))` and `datastar(Method(...))` would select a
frontend-specific template data type per route, `--output-htmx`/`--output-datastar`
would wrap every unframed route, and the two could mix in one package.

Prototyping the design surfaced a lot of incidental complexity: wrapper arity
errors, auto-wrap versus explicit-wrap interactions, conditional emission of
the base `TemplateData` when every route is framed, and a breaking move of the
HX* helpers off the shared type. The wrapper also duplicates a decision the
template body already makes — markup written with `hx-*` or `data-*`
attributes commits the file to a library, and a per-route name wrapper can
drift from the body it names.

A project that genuinely serves both libraries already has an idiomatic
answer: one package per frontend, each with its own template set and its own
`muxt generate` invocation, registering routes on a shared `http.ServeMux`.

## Decision

Select the frontend library per package with generate flags
(`--output-htmx`, and later `--output-datastar`); do not add framing wrappers to
the template name syntax. Mixing frontends means multiple packages sharing a
mux.

The flags extend the existing generated types with library-specific surface:
`--output-htmx` adds the HX* helper methods to `TemplateData`, and
`--output-datastar` extends `SSETemplateData` with the patch-protocol event
surface. No parallel library-specific template data types are generated.

## Status

Decided

## Consequences

- The template name grammar stays a single call expression; representation
  wrappers (`sse`, `marshalJSON`) are unaffected.
- The HX* helpers stay on the shared `TemplateData`, so no breaking type
  split or migration is needed; `--output-htmx` supersedes the name
  `--output-htmx-helpers`.
- Datastar support becomes package-level configuration: `--output-datastar`
  extends `SSETemplateData` with the patch option setters and the
  patch-protocol wire format, keyed off the generate configuration rather
  than per-route state. Rendered (non-SSE) routes are unchanged by it.
- Per-route mixing inside one package is not supported, and `muxt check`
  cannot flag an htmx helper call in a package generated for another
  frontend; the package boundary provides that separation instead.
