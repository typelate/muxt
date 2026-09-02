# Datastar Search

Live search over the Go proverbs, driven by [Datastar](https://data-star.dev) signals. It shows why the request-body JSON binding exists: Datastar sends the page's signal state as a JSON request body, and `unmarshalJSON(body)` decodes it — signals need no dedicated muxt feature.

## How it works

`data-bind:query` creates the signal; typing posts it (debounced) to the search route:

```gotmpl
<input type="search" data-bind:query
       data-on:input__debounce.300ms="@post('{{.Path.SearchProverbs}}')">
```

The route decodes the signals from the body and streams one patch with the results and footer:

```gotmpl
{{define "POST /search sse(SearchProverbs(ctx, unmarshalJSON(body), sseResults))"}}{{end}}
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
