# Datastar Counter Example

A minimal [Datastar](https://data-star.dev) app generated with `muxt --use-datastar`,
exercising all four Datastar response representations.

```sh
go generate ./...
go run .
# open http://localhost:8000
```

## What it demonstrates

| Feature | Route | Datastar |
|---|---|---|
| **Signals** (`application/json`) | `POST /increment Increment(ctx, signal)` | `@post('/increment')` merges `{"count":N}` into `$count` |
| **Element patch** (SSE) | `GET /clock Clock(ctx, elements)` | `data-init="@get('/clock')"` streams `datastar-patch-elements` into `#clock` |
| **Script** (`text/javascript`) | `GET /hello.js Hello(ctx, script)` | `@get('/hello.js')` executes the rendered JS |
| **Actions** | — | `{{ (.Actions.Increment).JS }}` renders the `@post(...)` expression |

## Notes

- Datastar v1 event attributes use a **colon**: `data-on:click` (not `data-on-click`).
- Render Datastar action expressions with `.JS` (returns `template.JS`) inside
  `data-on:*` attributes; `html/template` parses those as a JavaScript context
  and would otherwise JSON-wrap a plain string.
- The committed `template_routes.go` marshals signals with `encoding/json`. When
  generated under `GOEXPERIMENT=jsonv2` it uses `encoding/json/v2` `MarshalWrite`
  instead (requires a `go 1.25`+ module).

See [docs/reference/datastar.md](../../reference/datastar.md) for the full reference.
