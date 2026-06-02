# 10 - Request Input Binding via body and unmarshalX Wrappers

## Context

muxt's `form` argument parses `request.Form`, but there is no way to bind a JSON
request body to a struct. Datastar `@post` sends a JSON signals body, which forced
the example to use `contentType: 'form'`. The grammar already supports nested call
arguments, so a decoding wrapper fits naturally.

## Decision

Add a reserved **`body`** identifier of type `io.Reader` (the request body) and
argument wrappers that decode it into the **Go type of the method parameter at
that call position**:

- **`unmarshalJSON(body)`** — `io.ReadAll` + `json.Unmarshal`; under
  `--output-jsonv2`, `encoding/json/v2` `UnmarshalRead(body, &v)`.
- **`unmarshalForm(body)`** — decode the form-encoded body. The existing **`form`
  argument on non-GET methods is syntactic sugar** for `unmarshalForm(body)` (on
  GET, `form` binds the query string, a different source, so the equivalence is
  non-GET only).
- **`signals`** — Datastar sugar for `unmarshalJSON(body)`.

`unmarshalXML` is possible later but not planned.

## Status

Decided

## Consequences

One coherent input model: the `unmarshalX` wrappers are the mechanism and existing
input args are sugar over them. `marshalJSON` (see the direction spec) is the
output dual. The decoder target type comes from the receiver method signature, so
`muxt check` can type-check it.
