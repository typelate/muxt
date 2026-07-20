# HTMX Helpers: Counter

A counter you increment and decrement over HTMX. It demonstrates the `--output-htmx-helpers` flag, which generates HTMX header methods on `TemplateData` so you can set and read HTMX headers from inside a template.

## Run it

```bash
go generate ./...
go run .
```

Open [http://localhost:8000](http://localhost:8000). Set `PORT` to use a different port.

## How the helpers are generated

`main.go` carries the directive:

```go
//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --output-htmx-helpers
```

`--output-htmx-helpers` adds methods to the generated `TemplateData`: response-header setters (`HXLocation`, `HXPushURL`, `HXRedirect`, `HXReswap`, `HXRetarget`, `HXTrigger`, …) and request-header readers (`HXRequest`, `HXBoosted`, `HXTriggerElementID`, …). This example's `POST /count` template calls one:

```gotmpl
{{- if eq .HXTriggerElementID "decrement"}}
  {{- template "count" .Receiver.Decrement}}
{{- else if eq .HXTriggerElementID "increment"}}
  {{- template "count" .Receiver.Increment}}
{{- end}}
```

`htmx_test.go` exercises each one against the generated type. Earlier muxt versions had no flag — you copied these methods in by hand; the flag replaces that.

## Routes

| Template name | Receiver method |
|---------------|-----------------|
| `/ Count()` | `Count() int64` |
| `/increment-count Increment()` | `Increment() int64` |
| `/decrement-count Decrement()` | `Decrement() int64` |
| `POST /count` | none — the template dispatches on `.HXTriggerElementID` to `.Receiver.Increment`, `.Receiver.Decrement`, or `.Receiver.Count` |

Both page buttons post to `/count`; the template reads which button triggered the request from the `HX-Trigger` header helper and calls the matching receiver method — that dispatch is the point of the example. The `/increment-count` and `/decrement-count` routes expose the same operations as standalone endpoints. The `Server` holds the count in an `int64` it reads and updates with `sync/atomic`.