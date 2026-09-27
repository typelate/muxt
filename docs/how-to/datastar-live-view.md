# How to build a Datastar live view

Generate with `--output-datastar`. Server-Sent Events routes then emit Datastar patch events. The [datastar-counter](../examples/datastar-counter), [datastar-todo](../examples/datastar-todo), and [datastar-search](../examples/datastar-search) examples cover every section below except the patch setters.

## Patch elements

Each `sse`-prefixed callback renders the template of the same name into one `datastar-patch-elements` event ([sse callbacks](../reference/call-parameters.md#server-sent-events)):

```gotmpl
{{define "sseCount"}}<output id="count">{{.Result}}</output>{{end}}
{{define "POST /increment sse(Increment(ctx, sseCount, deltaSignals))"}}{{end}}
```

```go
func (s *Server) Increment(_ context.Context, sseCount func(int64) error, deltaSignals func(Delta) error)
```

Datastar morphs the element by id. When id matching is not enough, the fragment template calls `.Selector`, `.Mode`, or `.UseViewTransition` on the dot; [reference_output_datastar_elements.txt](../../cmd/muxt/testdata/reference_output_datastar_elements.txt) shows `.Selector` and `.Mode`.

## Patch signals

`deltaSignals` above is a `Signals`-suffixed callback ([reference](../reference/call-parameters.md#datastar)). Its argument is marshaled as JSON into a `datastar-patch-signals` event:

```go
type Delta struct{ Delta string `json:"delta"` }
```

The struct's `json` tags must match the page's `data-signals` keys; muxt does not check this, so a typo fails silently in the browser. The wire format is asserted in [reference_output_datastar_signals_events.txt](../../cmd/muxt/testdata/reference_output_datastar_signals_events.txt).

Inbound, the `signals` argument binds the posted body on non-GET routes ([arguments](../reference/call-parameters.md#arguments)):

```gotmpl
{{define "POST /search sse(SearchProverbs(ctx, signals, sseResults))"}}{{end}}
```

## Use .Path inside Datastar attributes

`html/template` treats `data-on:*` values as JavaScript, so a generated path renders escaped (`\/increment`) and evaluates to the same string in the browser:

```gotmpl
<button data-on:click="@post('{{.Path.Increment}}')">+</button>
```

Inside `{{range}}` use `$.Path`: `{{$.Path.ToggleTodo .ID}}`.

## Test the wire contract

Drive the generated routes with `httptest` and assert on the events. The `patchElements` helper in the [datastar-todo test](../examples/datastar-todo/template_test.go) parses a patch event into a [domtest](https://pkg.go.dev/github.com/typelate/dom/domtest) fragment.
