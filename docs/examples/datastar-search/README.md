# Datastar Search

Live search over the Go proverbs, driven by [Datastar](https://data-star.dev) signals. Datastar sends the page's signal state as a JSON request body; the reserved `signals` argument (available under `--output-datastar`) decodes it — it is shorthand for `unmarshalJSON(body)`, so both spellings bind identically.

## How it works

`data-bind:query` creates the signal; typing posts it (debounced) to the search route:

```gotmpl
<input type="search" data-bind:query
       data-on:input__debounce.300ms="@post('{{.Path.SearchProverbs}}')">
```

The route decodes the signals from the body and streams one patch with the results and footer:

```gotmpl
{{define "POST /search sse(SearchProverbs(ctx, signals, sseResults))"}}{{end}}
```

```go
type SearchSignals struct {
	Query string `json:"query"`
}

func (Server) SearchProverbs(_ context.Context, signals SearchSignals, sseResults func(SearchResults) error) {
	_ = sseResults(search(signals.Query))
}
```

Matches split around the hit so the highlight is server-rendered (`{{.Prefix}}<mark>{{.Match}}</mark>{{.Suffix}}`), and the same search is exposed as JSON with the `marshalJSON(...)` wrapper — one receiver, two representations:

```gotmpl
{{define "GET /api/proverbs marshalJSON(ProverbsAPI(ctx, form))"}}{{end}}
```

## Run it

```bash
go generate ./...
go run .
# open http://localhost:8003
```

The tests are Given/When/Then subtests over [domtest](https://pkg.go.dev/github.com/typelate/dom/domtest), covering the page affordances, the signal round trip (including case-insensitivity, misses, and a malformed body responding 400), and the JSON endpoint.
