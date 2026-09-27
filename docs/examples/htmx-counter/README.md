# htmx Counter

A counter you increment and decrement over htmx. It shows [`--output-htmx`](../../reference/cli.md#generate-output-flags), which adds htmx header methods to the generated `TemplateData`.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8000. `PORT` overrides the port.

## Read in this order

1. [template.gohtml](template.gohtml): the `POST /count` template reading `.HXTriggerElementID` to choose between `.Receiver.Increment`, `.Receiver.Decrement`, and `.Receiver.Count`.
2. [main.go](main.go): `Server` keeping the count in an `int64` guarded by `sync/atomic`, and the `//go:generate` directive passing `--output-htmx`.
3. [htmx_test.go](htmx_test.go): a subtest for each generated header method.
4. [template_test.go](template_test.go): `muxt check` run as a test.

## Routes

| Template name | Method |
|---|---|
| `/ Count()` | `Count() int64` |
| `/increment-count Increment()` | `Increment() int64` |
| `/decrement-count Decrement()` | `Decrement() int64` |
| `POST /count` | none |

The `POST /count` dispatch is the point of the example, not a recommended design. The `/increment-count` and `/decrement-count` routes show the plain alternative and render a full page when `.HXRequest` is false.
