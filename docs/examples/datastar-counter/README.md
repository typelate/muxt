# Datastar Showcase Example

A [Datastar](https://data-star.dev) app generated with `muxt --use-datastar`,
exercising every Datastar response representation and most muxt features on one
page.

```sh
go generate ./...
go run .
# open http://localhost:8000
```

## Datastar / muxt features covered

| Area | Where |
|---|---|
| **Signals** (`application/json`) over `@post`/`@put`/`@patch`/`@delete` | `Increment`, `Decrement`, `Reset`, `Clear`, `Adjust` |
| **Path parameter** (typed `int`) | `PATCH /count/{delta} Adjust` → `@patch('/count/5')` |
| **Element patch** (SSE, inner mode + view transition) | `GET /clock Clock(ctx, lastEventID, elements)` |
| **lastEventID** (SSE resume wiring) | `Clock` |
| **Append mode + multiple callbacks + inline patch-signals + onlyIfMissing** | `GET /feed Feed(ctx, elements, signalStatus)` |
| **Form binding** + action option `contentType: 'form'` | `POST /greet Greet(ctx, form, elements)` |
| **Script** (`text/javascript`) with a JSON body via the `json` func | `GET /config.js Config(ctx, script)` |
| **Fragment** (`text/html`) + status code + `request` | `GET /fragment 200 Fragment(ctx, request)` |
| **`.Actions()`** for every verb, fluent options, `.JS` / `String()` | the `Index` template |
| **`.Path()`** helper | the `Index` template |

Client attributes used: `data-signals`, `data-text`, `data-bind`,
`data-computed`, `data-show`, `data-class`, `data-indicator`, `data-init`,
`data-on:*` (and `data-on:submit__prevent`).

## Notes

- Datastar v1 events use a **colon**: `data-on:click`. Render actions there with
  `.JS` (`template.JS`); `String()` (the default `{{ .Actions.X }}`) is correct
  everywhere else (`data-init`, plain attributes, text).
- The `json` template func returns `template.HTML` so the `script` route can
  embed server data as JSON (`encoding/json` already escapes `<>&`).
- Signals marshal with the standard library `encoding/json`. Pass
  `--output-jsonv2` to `muxt generate` to use `encoding/json/v2` `MarshalWrite`
  instead (only for modules built with `GOEXPERIMENT=jsonv2`).

See [docs/reference/datastar.md](../../reference/datastar.md) for the reference.
