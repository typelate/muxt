# How to build a Datastar live view

Generate with `--output-datastar`. The flag frames Server-Sent Events with
Datastar's patch-elements protocol and enables the signals bindings in both
directions. The [datastar-counter](../examples/datastar-counter),
[datastar-todo](../examples/datastar-todo), and
[datastar-search](../examples/datastar-search) examples run everything on this
page.

## Stream element patches

An sse route's callbacks each render a named template into a patch event. The
callback argument names the template it renders:

```gotmpl
{{define "sseCount"}}<output id="count">{{.Result}}</output>{{end}}

{{define "POST /increment sse(Increment(ctx, sseCount))"}}{{end}}
```

```go
func (s *Server) Increment(_ context.Context, sseCount func(int64) error) {
	_ = sseCount(s.count.Add(1))
}
```

On the wire, each callback invocation is one event:

```
event: datastar-patch-elements
data: elements <output id="count">1</output>
```

Datastar morphs the element in by id. Use the `SSETemplateData` setters
(`.Selector`, `.Mode`, `.UseViewTransition`) inside the fragment template when
id-matching is not enough.

## Patch signals

Outbound, a `Signals`-suffixed callback marshals its argument as a
`datastar-patch-signals` event instead of rendering a template — the suffix
selects the behavior and the shape must be exactly `func(T) error`:

```go
func (s *Server) Increment(_ context.Context, sseCount func(int64) error, deltaSignals func(Delta) error) {
	_ = sseCount(s.count.Add(1))
	_ = deltaSignals(Delta{Delta: "+1"})
}
```

```
event: datastar-patch-signals
data: signals {"delta":"+1"}
```

The JSON keys come from `T`'s `encoding/json` tags and must match the page's
`data-signals` attribute keys — muxt does not check this contract, so a typo'd
signal key fails silently in the browser. Keep `T` next to the templates and
give every field a json tag.

Inbound, the reserved `signals` argument binds the signal state the browser
posts; it is shorthand for `unmarshalJSON(body)` and decodes into the
parameter's type with a 400 on malformed input.

A route may combine all three: `sse(Redeploy(ctx, id, signals, flashSignals))`.
An sse route may also omit the render callback entirely — the stream then
carries only signal patches.

## Link with .Path inside Datastar attributes

html/template treats `data-on:*` values as JavaScript, so a generated path
renders escaped (`\/increment`) but evaluates to the same string in the
browser:

```gotmpl
<button data-on:click="@post('{{.Path.Increment}}')">+</button>
```

Inside a `{{range}}` the dot is the element, so reach the helpers through the
root: `{{$.Path.Redeploy .ID}}`. A partial rendered with `{{template "row" .}}`
receives only the value you pass it — pass the whole template data (or the
pre-rendered URL) when the partial needs `.Path`.

## Test the wire contract

Drive the generated routes with `httptest` and assert on frames; the examples'
`template_test.go` files show a reusable `patchElements` assertion built on
[domtest](https://pkg.go.dev/github.com/typelate/dom/domtest).
