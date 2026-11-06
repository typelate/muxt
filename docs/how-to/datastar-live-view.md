# How to build a Datastar live view

Generate with `--output-datastar`; Server-Sent Events routes then emit Datastar patch events. Working code for most sections below is in [datastar-counter](../examples/datastar-counter), [datastar-todo](../examples/datastar-todo), and [datastar-search](../examples/datastar-search).

## Patch elements

Each `sse`-prefixed callback renders the template of the same name into one `datastar-patch-elements` event ([sse callbacks](../reference/call-parameters.md#server-sent-events)):

```gotmpl
{{define "sseCount"}}<output id="count">{{.Result}}</output>{{end}}
{{define "POST /increment sse(Increment(ctx, sseCount, deltaSignals))"}}{{end}}
```

```go
func (s *Server) Increment(_ context.Context, sseCount func(int64) error, deltaSignals func(Delta) error)
```

Datastar morphs the element by id.

## Append a toast with a second callback

To patch more than one part of the page, pass several `sse` callbacks; each renders its own template. This save appends the new row, then a toast:

```gotmpl
{{define "POST /items sse(Save(ctx, form, sseRow, sseToast))"}}{{end}}

{{define "sseRow"}}{{.Selector "#items"}}{{.Mode "append"}}{{template "row" .Result}}{{end}}

{{define "sseToast"}}{{.Selector "#toasts"}}{{.Mode "append"}}<p class="toast" role="status">{{.Result}}</p>{{end}}

{{define "row"}}<li id="item-{{.ID}}">{{.Title}}</li>{{end}}
```

```go
func (s *Server) Save(ctx context.Context, form ItemForm, sseRow func(Item) error, sseToast func(string) error) error {
	item, err := s.items.Add(ctx, form.Title)
	if err != nil {
		return err
	}
	if err := sseRow(item); err != nil {
		return err
	}
	return sseToast("Saved " + item.Title)
}
```

A new row and a toast have no element on the page yet, so matching by id has nothing to morph. `.Selector` names the element to patch and `.Mode "append"` adds the fragment as its last child. Both print nothing. The page holds the empty containers:

```html
<ul id="items"></ul>
<div id="toasts" aria-live="polite"></div>
```

The two calls arrive as two events:

```text
event: datastar-patch-elements
data: selector #items
data: mode append
data: elements <li id="item-1">Milk</li>

event: datastar-patch-elements
data: selector #toasts
data: mode append
data: elements <p class="toast" role="status">Saved Milk</p>
```

The fragment templates get `*SSETemplateData`, so they read `.Result.ID`, not `.ID`. `row` is a plain template both the page and `sseRow` render.

## Patch signals

`deltaSignals` above is a `Signals`-suffixed callback ([reference](../reference/call-parameters.md#datastar)). Its argument is marshaled as JSON into a `datastar-patch-signals` event:

```go
type Delta struct{ Delta string `json:"delta"` }
```

The struct's `json` tags must match the page's `data-signals` keys. The wire format is asserted in [reference_output_datastar_signals_events.txt](../../cmd/muxt/testdata/reference_output_datastar_signals_events.txt).

Inbound, the `signals` argument binds the posted body on non-GET routes ([arguments](../reference/call-parameters.md#arguments)):

```gotmpl
{{define "POST /search sse(SearchProverbs(ctx, signals, sseResults))"}}{{end}}
```

## Serve a fragment and a full page from one GET route

A route need not stream. Datastar patches a `text/html` response by element id, so a plain GET route can answer an `@get` action with a fragment and a direct visit with the whole page. `.DatastarRequest` is true when the request carries `Datastar-Request: true`, which every Datastar action sends:

```gotmpl
{{define "GET /list List(ctx)"}}
{{- with .Header "Vary" "Datastar-Request"}}
{{- if .DatastarRequest}}{{template "list" .Result}}
{{- else}}<!DOCTYPE html><html lang="en"><body>{{template "list" .Result}}</body></html>{{end}}
{{- end}}
{{- end}}
{{define "list"}}<ul id="list">{{range .}}<li>{{.}}</li>{{end}}</ul>{{end}}
```

`Vary` keeps a cache from serving the fragment to a direct visit. [reference_output_datastar_request.txt](../../cmd/muxt/testdata/reference_output_datastar_request.txt) asserts both responses.

## Use .Path inside Datastar attributes

`html/template` treats `data-on:*` values as JavaScript, so a generated path renders escaped (`\/increment`) and evaluates to the same string in the browser:

```gotmpl
<button data-on:click="@post('{{.Path.Increment}}')">+</button>
```

## Test the wire contract

Drive the generated routes with `httptest` and assert on the events. The `patchElements` helper in the [datastar-todo test](../examples/datastar-todo/template_test.go) parses a patch event into a [domtest](https://pkg.go.dev/github.com/typelate/dom/domtest) fragment.

## Edge cases

- **More than id matching.** When matching by id is not enough, the fragment template calls `.Selector`, `.Mode`, or `.UseViewTransition` on the dot, as [the toast](#append-a-toast-with-a-second-callback) does. [reference_output_datastar_elements.txt](../../cmd/muxt/testdata/reference_output_datastar_elements.txt) asserts the `selector` and `mode` lines.
- **Callback order is event order.** Each callback writes and flushes its event when called, so `sseRow` before `sseToast` means the row lands first. When `sseRow` returns an error, the client is gone or the template failed, so `Save` returns without the toast.
- **Signal names are not checked.** muxt does not compare the `json` tags with the page's `data-signals` keys, so a typo fails silently in the browser.
- **`.Path` inside `{{range}}`.** Use `$.Path`, as in `{{$.Path.ToggleTodo .ID}}`.
