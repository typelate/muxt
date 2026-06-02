# 8 - The sse() Wrapper Marks Streaming; send/sendX Callbacks Render Events

## Context

SSE endpoints are flagged today by a reserved `sse` callback argument, and extra
streams use an `sse`-prefix naming convention (`sseClock` → template `sseClock`).
`execute` is a separate reserved callback that lets a non-SSE handler control when
its template renders.

## Decision

Mark an SSE endpoint with the **`sse(Call(...))`** representation wrapper (an
unwrapped call is an ordinary `ExecuteTemplate` handler). Inside `sse(...)` the
handler is either:

- a **channel or iterator** return — `<-chan T`, `iter.Seq[T]`, `iter.Seq2[T,
  error]` — where each value is one event rendered via the route's `define` body;
  an `iter.Seq2` error replaces the entries on the event template-data error list;
  channels carry values only (no error form); or
- an **error/nothing** return that takes a **`send`** callback (renders the
  `define` body) plus optional **`sendIdentifier`** callbacks that render
  same-named templates (`sendClock` → template `Clock`).

A send callback may be wrapped: `marshalJSON(sendStatus)` marshals its value
instead of rendering a template (a `patch-signals` frame under Datastar), so one
stream can carry both element and signal senders.

Use **`send`** (uniform with `sendX`) for the SSE base callback; **`execute`** is
retained unchanged for non-SSE handlers.

## Status

Decided

## Consequences

The reserved `sse` parameter is no longer needed. `sendX` names match templates
directly, so the same template is reusable via `{{template "Clock"}}`. The
`sse`/`sseX` arg forms (dev pre-releases only) are replaced cleanly.
