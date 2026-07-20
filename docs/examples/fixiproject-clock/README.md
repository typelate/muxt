# fixi + SSE Clock

A server clock that streams the current time once a second over Server-Sent Events, rendered with [fixi](https://www.npmjs.com/package/the-fixi-project). It demonstrates the `sse(...)` template-name wrapper and the `lastEventID` parameter, with no third-party SSE library on either side.

## Run it

```bash
go generate ./...
go run .
```

Open [http://localhost:8080](http://localhost:8080). Set `PORT` to use a different port.

## Routes

| Template name | Receiver method |
|---------------|-----------------|
| `GET /{$} Index()` | `Index() string` |
| `GET /time sse(Time(ctx, lastEventID, execute))` | `Time(context.Context, string, func(string) error)` |

Wrapping the call in `sse(...)` makes `Time` an SSE handler. Muxt establishes the event stream, then hands the method a closure at the `execute` argument's position that renders the route template into one SSE frame per call. The method loops on a one-second ticker, calling the closure with each new timestamp until the request context is cancelled.

`lastEventID` is bound from the `Last-Event-Id` request header — the value a browser replays when reconnecting a stream. See [Call Parameters](../../reference/call-parameters.md).

## How fixi drives it

The page loads the fixi script from a CDN, then wires the stream on the body:

```html
<body fx-action='{{.Path.Time}}' fx-trigger="fx:inited" fx-swap='innerHTML'>
{{block `GET /time sse(Time(ctx, lastEventID, execute))` .}}
    <time>{{.Result}}</time>
{{end}}
</body>
```

`fx-action='{{.Path.Time}}'` points at the `/time` route using the generated URL builder, so the link survives a route rename. `fx-trigger="fx:inited"` opens the stream as soon as fixi initialises, and each SSE frame replaces the body's inner HTML. A small `fx:config` listener turns on `sseReconnect` and `ssePauseOnHidden`.