# Datastar Counter

A counter you increment and decrement over [Datastar](https://data-star.dev). It shows the [`--output-datastar`](../../reference/cli.md#generate-output-flags) flag: each click streams one `datastar-patch-elements` event and one `datastar-patch-signals` event.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8001. `PORT` overrides the port.

## Read in this order

1. [template.gohtml](template.gohtml): the `sseCount` fragment, the buttons posting through `{{.Path.Increment}}` and `{{.Path.Decrement}}`, and `data-signals-delta` plus `data-text="$delta"` showing the signal patch.
2. [main.go](main.go): `Increment` calling `sseCount` with the new count and `deltaSignals` with the signal patch ([Datastar callbacks](../../reference/call-parameters.md#datastar)).
3. [template_test.go](template_test.go): `readEventStream` and `elementsFragment`, so the same selectors work on the page and on a patch.

## Routes

| Template name | Method |
|---|---|
| `GET / Home(ctx)` | `Home(context.Context) (int64, error)` |
| `POST /increment sse(Increment(ctx, sseCount, deltaSignals))` | `Increment(context.Context, func(int64) error, func(Delta) error)` |
| `POST /decrement sse(Decrement(ctx, sseCount, deltaSignals))` | `Decrement(context.Context, func(int64) error, func(Delta) error)` |
