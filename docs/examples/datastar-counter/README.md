# Datastar Counter

A counter you increment and decrement over [Datastar](https://data-star.dev). It demonstrates the `--output-datastar` flag, which frames Server-Sent Events with the Datastar patch-elements protocol.

## How it works

The page renders the count through the `sseCount` template. Each button posts a Datastar action:

```gotmpl
<button id="increment" data-on:click="@post('/increment')">+</button>
```

The route streams one patch event whose payload is the same `sseCount` fragment — the `sseCount` callback argument names the template it renders:

```gotmpl
{{define "sseCount"}}<output id="count">{{.Result}}</output>{{end}}

{{define "POST /increment sse(Increment(ctx, sseCount))"}}{{end}}
```

```go
func (s *Server) Increment(_ context.Context, sseCount func(int64) error) {
	_ = sseCount(s.count.Add(1))
}
```

On the wire that is:

```
event: datastar-patch-elements
data: elements <output id="count">1</output>
```

Datastar morphs the element in by id — no selector needed.

## Run it

```bash
go generate ./...
go run .
# open http://localhost:8001
```

The tests parse the page and the patch payloads with [domtest](https://pkg.go.dev/github.com/typelate/dom/domtest); `patchElements` in [template_test.go](template_test.go) is the reusable assertion for the wire contract.
