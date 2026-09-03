# 7 - Import SSE Libraries Instead of Generating the Wire Layer

## Context

Muxt generates the entire Server-Sent Events wire layer: stream headers, the
`http.Flusher` assertion, a `sync.Mutex`, the `SSETemplateData.WriteTo` frame
writer with forbidden-character checks, the datastar patch-elements and
patch-signals framing, and the `Last-Event-Id` plumbing. Generating everything
keeps the default dependency-free, but two robust protocol implementations
already exist:

- [github.com/typelate/sse](https://github.com/typelate/sse) — an
  allocation-light WHATWG SSE implementation. `sse.New(response, request,
  code)` owns headers, flushing, and goroutine safety; `Response.Message(data,
  ...MessageOption)` writes frames with `WithEvent`, `WithID`, `WithIntID`,
  and `WithRetry` options; `Response.LastEventID()` reads the reconnect
  header; `ErrInvalidField` covers field validation. It is frontend-agnostic:
  for datastar routes muxt still formats the `elements ...` payload lines and
  passes `WithEvent("datastar-patch-elements")`.
- [github.com/starfederation/datastar-go](https://github.com/starfederation/datastar-go)
  — the official Datastar SDK. `datastar.NewSSE(w, r, ...SSEOption)` plus
  `PatchElements(html, ...PatchElementOption)` and
  `MarshalAndPatchSignals(v, ...PatchSignalsOption)` replace both generated
  closure kinds and bring capabilities muxt does not generate: response
  compression, `onlyIfMissing` signal patches, `ExecuteScript`, and
  `ReadSignals`, which also decodes signals from the `?datastar=` query on GET
  requests — a case the `signals` argument (sugar for `unmarshalJSON(body)`)
  misses.

Neither existing flag family fits: `--use-*` names source queries and
`--output-*` names generated identifiers ([decision 5](00005_rename_flags.md)),
but this choice is about which third-party module the generated code depends
on.

## Decision

Add an `--import-*` flag family: an `--import-<module>` flag means the
generated code imports that module instead of generating the equivalent code.
The default remains the generated, dependency-free implementation.

- `--import-typelate-sse` — valid with any frontend (vanilla, `--output-htmx`,
  `--output-datastar`). Replaces stream setup, the mutex, frame writing,
  field validation, and `Last-Event-Id` handling with `sse.New` and
  `Response.Message`. The `SSETemplateData` event setters map to
  `MessageOption` values.
- `--import-star-federation-datastar` — requires `--output-datastar` and is
  mutually exclusive with `--import-typelate-sse`. Replaces the render and
  signals closures with `PatchElements` and `MarshalAndPatchSignals`, decodes
  the `signals` argument with `ReadSignals`, and exposes stream-level
  `SSEOption` values (compression among them).

Library capabilities reach receiver methods through variadic option
parameters on the callback signatures. Under an import flag, a callback
parameter may be declared with the library's option type for its kind, and
the generated closure forwards the options to the library call:

```go
// --import-typelate-sse
func (s Server) Watch(ctx context.Context, sseCount func(int64, ...sse.MessageOption) error) error

// --import-star-federation-datastar
func (s Server) Increment(ctx context.Context,
	sseCount func(int64, ...datastar.PatchElementOption) error,
	deltaSignals func(Delta, ...datastar.PatchSignalsOption) error,
) error
```

Validation accepts exactly `func(T) error` or `func(T, ...O) error`, where
`O` is type-checked against the imported module's option type for that
callback kind (`MessageOption` for render callbacks under typelate-sse;
`PatchElementOption` for render callbacks and `PatchSignalsOption` for
Signals-suffixed callbacks under star-federation-datastar). Without an import
flag the variadic form is a generation error naming the flag that permits it.

Stream-level options that precede any callback (compression, per-stream
configuration) are supplied by the receiver when it implements an optional
interface the generated handler asserts at generation time; the exact
interface shape is settled in the implementing change.

## Status

Decided

## Consequences

- The default stays dependency-free; projects opt into a module dependency
  explicitly, and the flag name says which module the generated code will
  import.
- Receiver signatures using variadic options couple to the chosen import
  flag: switching libraries is a compile-time break in receiver code. This is
  accepted — exposing a library's own option types is the point, and wrapping
  them in muxt-owned types would re-implement the surface the flags exist to
  avoid.
- `--import-star-federation-datastar` closes the `ReadSignals` GET-query gap
  and the compression gap; `muxt check` and generation validate option types
  against the loaded module, so the module must be in the project's go.mod.
- The generated-code paths triple for SSE routes (generated, typelate-sse,
  star-federation-datastar); txtar coverage is required per path, and the
  generated default remains the reference behavior the libraries must match.
- `--import-typelate-sse` removes the generated mutex: `sse.Response`
  serializes concurrent `Message` calls itself.
