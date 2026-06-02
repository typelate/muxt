# 7 - Framing Wrappers Select the Template Data Type and Event Marshaler

## Context

`--use-htmx` mutates `TemplateData` by adding HX* methods, and the Datastar work
added a parallel `DatastarTemplateData`. Frontends frame responses differently
(Datastar prefixes SSE events; HTMX/Fixi use plain `data:`), so the choice of
frontend is really a choice of template-data type and event marshaler.

## Decision

Introduce outer **framing wrappers** `htmx(...)` and `datastar(...)` that select:

- the template-data type the handler renders with — `htmx(...)` →
  `HTMXTemplateData`, `datastar(...)` → `DatastarTemplateData`; and
- the SSE event marshaler (the `WriteTo` pattern).

Split the HX* helpers out of `TemplateData` into a dedicated `HTMXTemplateData`,
parallel to `DatastarTemplateData`; `TemplateData` stays minimal.

`--use-htmx` / `--use-datastar` wrap **every** route's call in the corresponding
framing wrapper. There is **no per-route opt-out** under the flag: to mix
frontends or leave routes unwrapped, omit the flag and write wrappers explicitly
per route.

## Status

Decided

## Consequences

`TemplateData` no longer changes shape by flag. Frontends are pluggable via their
marshaler. Apps that need more than one frontend drop the flag and wrap
explicitly. See [9](00009_event_framing_is_a_custom_marshaler.md) for why framing
is not generalized.
