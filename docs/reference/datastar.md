# Datastar Reference

Datastar mode generates [Datastar](https://data-star.dev) endpoints. Enable it with `--use-datastar` on both `muxt generate` and `muxt check`. It is mutually exclusive with `--use-htmx`.

In Datastar mode the generated template data types are renamed:

| Default mode | Datastar mode (`--use-datastar`) |
|---|---|
| `TemplateData` | `DatastarTemplateData` (adds `.Actions()`) |
| `SSETemplateData` | `DatastarEventTemplateData` (patch-elements frame) |

Both names remain overridable with `--output-template-data-type` / `--output-sse-template-data-type`.

## Render-callback arguments

These call arguments are recognized only under `--use-datastar`. Each supports camelCase-prefixed variants (`elementsClock`, `signalCount`) that render a same-named template.

| Argument | Response | Frame / body |
|---|---|---|
| `elements` | `text/event-stream` | `event: datastar-patch-elements` |
| `signal` | `application/json` standalone, or inline `datastar-patch-signals` when the route streams | `data: signals <json>` |
| `script` | `text/javascript` | rendered template body |

The response representation is fixed per route by the declared arguments:

- any `elements` → streaming `text/event-stream` (a `signal` argument on the same route emits inline `datastar-patch-signals` frames);
- a lone `signal` → `application/json`;
- a lone `script` → `text/javascript`;
- none → `text/html` fragment (unchanged).

The generic `sse(...)` representation wrapper (see [Call Parameters Reference](call-parameters.md#sse)) emits plain `data:`-framed SSE events and works regardless of `--use-datastar`; Datastar `patch-elements`/`patch-signals` framing of `sse(...)` arrives in a later phase. For Datastar-framed streaming, use `elements` for now.

### elements

```gotemplate
{{define "GET /events Stream(ctx, lastEventID, elements)"}}{{.Selector "#target"}}{{.Mode "inner"}}{{.Result}}{{end}}
```

```go
func (s Server) Stream(ctx context.Context, lastEventID string, elements func(string) error) {
	_ = elements("hello-" + lastEventID)
}
```

`DatastarEventTemplateData` adds chainable setters used from inside the template, alongside `.Result`, `.Request`, `.Path`, `.Err`:

| Method | Wire line | Default (omitted) |
|---|---|---|
| `.Selector(string)` | `data: selector <v>` | none |
| `.Mode(string)` | `data: mode <v>` | `outer` |
| `.Namespace(string)` | `data: namespace <v>` | none |
| `.UseViewTransition(bool)` | `data: useViewTransition true` | `false` |

Each rendered line is emitted as its own `data: elements <line>`.

### signal

The `signal` callback is `func(data T, onlyIfMissing bool) error`. It marshals `data` to JSON; it does not render a template. `onlyIfMissing` is emitted as `data: onlyIfMissing true` only on streaming routes — a standalone `application/json` body cannot carry it, so it is ignored there.

```gotemplate
{{define "POST /count Increment(ctx, signal)"}}{{end}}
```

```go
func (s Server) Increment(ctx context.Context, signal func(Count, bool) error) {
	_ = signal(Count{Count: 5}, false)
}
```

By default the marshal helper uses the standard library `encoding/json` `Marshal`. Pass `--output-jsonv2` to emit `encoding/json/v2` `MarshalWrite` (into the pooled buffer) instead — only do this for modules built with `GOEXPERIMENT=jsonv2` (requires a `go 1.25`+ module). The generator never references the `go-json-experiment` backport.

### script

The `script` callback renders the same-named template into a `text/javascript` body. To run a script mid-stream, append a `<script>` element via `elements`.

## Actions

In Datastar mode `DatastarTemplateData` gains an `.Actions()` accessor — a parallel to `.Path()` — with one method per route. Each method takes the same path parameters as the route and returns a `DatastarAction` whose HTTP verb is inferred from the route's method (`GET`→`@get`, `POST`→`@post`, `PUT`→`@put`, `PATCH`→`@patch`, `DELETE`→`@delete`).

`DatastarAction` implements both `String()` and `JS()` (returning `template.JS`).

`String()` (the default when you write `{{ .Actions.X }}`) is correct in every
context **except** Datastar `data-on:*` event-handler attributes. Datastar v1
events use a colon — `data-on:click`, `data-on:submit` — and `html/template`
parses those as a JavaScript context, where it would JSON-wrap a plain string
into `"@post('/x')"` (a quoted literal Datastar will not execute). There, use the
`.JS` terminal:

```gotemplate
<!-- data-on:* (JS context) -> needs .JS -->
<button data-on:click="{{ (.Actions.UpdateUser .ID).JS }}">save</button>
<a data-on:click="{{ ((.Actions.UpdateUser .ID).OpenWhenHidden true).JS }}">defer</a>

<!-- data-init / data-bind / plain data-* / text -> String() is fine -->
<div data-init="{{ .Actions.Refresh }}"></div>
```

These render `@patch('/users/42')` and `@patch('/users/42', {openWhenHidden: true})`
(single quotes html-escaped to `&#39;`, which the browser decodes back to `'`).

Fluent option setters (each returns a copy, so an action can be reused):
`.ContentType(string)`, `.OpenWhenHidden(bool)`, `.Selector(string)`, `.Retry(string)`, `.RequestCancellation(string)`, `.RetryInterval(int)`, `.RetryScaler(float64)`, `.RetryMaxWait(int)`, `.RetryMaxCount(int)`. End an option chain with `.JS` in `data-on:*` contexts.

See the [Datastar SSE events](https://data-star.dev/reference/sse_events) and [backend actions](https://data-star.dev/reference/actions) references for the protocol.

## See also

- [Call Parameters Reference](call-parameters.md) — all call arguments.
