# fixi + SSE Clock

A clock that streams the time once a second over Server-Sent Events, rendered with [fixi](https://www.npmjs.com/package/the-fixi-project). It shows the [`sse(...)`](../../reference/call-parameters.md#server-sent-events) wrapper and a `{tz...}` wildcard route whose path helper escapes each segment. `Time` logs `lastEventID` and does not resume from it.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8080. `PORT` overrides the port.

## Read in this order

1. [index.gohtml](index.gohtml): the `<body>` tag's `fx-*` attributes, which open the stream on load and swap each frame in, and an `fx:config` listener turning on `sseReconnect` and `ssePauseOnHidden`.
2. [main.go](main.go): `Time`, looping on a ticker and calling the `execute` callback (`updateTime`) until the request context is cancelled or the callback returns an error.
3. [main_test.go](main_test.go): per-segment escaping in the `InZone` path helper.

## Routes

| Template name | Method |
|---|---|
| `GET /{$} Index()` | `Index() string` |
| `GET /time sse(Time(ctx, lastEventID, execute))` | `Time(context.Context, string, func(string) error)` |
| `GET /zone/{tz...} InZone(tz)` | `InZone(string) (string, error)` |
