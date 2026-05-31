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

`--output-htmx-helpers` adds methods to the generated `TemplateData`: response-header setters (`HXLocation`, `HXPushURL`, `HXRedirect`, `HXReswap`, `HXRetarget`, `HXTrigger`, …) and request-header readers (`HXRequest`, `HXBoosted`, `HXCurrentURL`, …). You call them from a template:

```gotmpl
{{.HXTrigger "count-changed"}}
```

`htmx_test.go` exercises each one against the generated type. Earlier muxt versions had no flag — you copied these methods in by hand; the flag replaces that.

## Routes

| Template name | Receiver method |
|---------------|-----------------|
| `/ Count()` | `Count() int64` |
| `/increment-count Increment()` | `Increment() int64` |
| `/decrement-count Decrement()` | `Decrement() int64` |
| `POST /count` | none (renders the `count` fragment) |

The `Server` holds the count in an `int64` it reads and updates with `sync/atomic`. Increment and decrement return the new value, which the route template renders back into the page.