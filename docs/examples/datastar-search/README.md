# Datastar Search

Live search over the Go proverbs, driven by [Datastar](https://data-star.dev) signals, generated with [`--output-datastar`](../../reference/cli.md#generate-output-flags). It shows the [`signals`](../../reference/call-parameters.md#arguments) argument and the same search served as JSON through [`marshalJSON`](../../reference/call-results.md#json-responses).

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8003. `PORT` overrides the port.

## Read in this order

1. [template.gohtml](template.gohtml): `data-bind:query` creating the signal and a debounced `data-on:input` posting it to the search route.
2. [main.go](main.go): `SearchSignals`, the JSON shape Datastar sends, and `search`, which splits each hit so the template can wrap it in `<mark>`.
3. [template_test.go](template_test.go): the signal round trip, case-insensitivity, a malformed body responding 400, and the JSON endpoint.

## Routes

| Template name | Method |
|---|---|
| `GET / Index(ctx)` | `Index(context.Context) (SearchResults, error)` |
| `POST /search sse(SearchProverbs(ctx, signals, sseResults))` | `SearchProverbs(context.Context, SearchSignals, func(SearchResults) error)` |
| `GET /api/proverbs marshalJSON(ProverbsAPI(ctx, form))` | `ProverbsAPI(context.Context, APIForm) (SearchResults, error)` |
