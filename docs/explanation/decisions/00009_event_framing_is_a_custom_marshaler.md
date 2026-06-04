# 9 - SSE Event Framing Is a Per-Frontend Marshaler, Not Generalized

## Context

It is tempting to generalize event rendering across frontends. But the SSE wire
syntax differs: Datastar prefixes events (`event: datastar-patch-elements`,
`data: selector …`, `data: elements …`, `data: signals …`), while HTMX and Fixi
use plain `data:` events with no such syntax. The current SSE/Datastar event
template-data types already encode this in their `WriteTo` methods.

## Decision

Keep per-event framing as a **custom event marshaler** (the `WriteTo` pattern),
selected by the framing wrapper:

- the generic `data:`-line marshaler (`SSETemplateData.WriteTo`) serves HTMX, Fixi,
  and raw SSE;
- Datastar's `patch-elements` / `patch-signals` marshalers
  (`DatastarEventTemplateData.WriteTo`) serve `datastar(...)`.

The SSE transport (event-stream headers, flush, the iterator/`send` mechanics) is
generic; only the framing is pluggable. Do not collapse framings into one renderer.

## Status

Implemented (Phase 2)

## Consequences

Each frontend supplies its marshaler; adding a frontend means adding a marshaler,
not changing the transport. Per-frame selection within a Datastar stream is done
by wrapping send callbacks (see [8](00008_sse_wrapper_and_send_callbacks.md)).
