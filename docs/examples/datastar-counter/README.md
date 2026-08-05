# Datastar: Counter and Greeter

A counter and a greeter over [Datastar](https://data-star.dev). It demonstrates the `--use-datastar` flag, which wraps every route in the `datastar(...)` framing: templates render with the Datastar template data types, streams speak the `datastar-patch-elements` protocol, the reserved `signals` argument binds client-sent signals, and `.Actions()` renders safe backend-action expressions.

## Run it

```bash
go generate ./...
go run .
```

Open [http://localhost:8001](http://localhost:8001). Set `PORT` to use a different port.

## How it works

`main.go` carries the directive:

```go
//go:generate go run github.com/typelate/muxt generate --use-receiver-type=Server --use-datastar
```

The page wires its buttons with generated actions — verb-correct, injection-proof `template.JS` expressions:

```gotmpl
<button id="increment" data-on:click="{{ .Actions.Increment | .JS }}">Increment</button>
```

`POST /increment` streams one `datastar-patch-elements` event: the `sendCount` callback renders the `Count` template (`sendX` renders the template named by its suffix), and Datastar swaps the fragment in by element id:

```gotmpl
{{define "POST /increment sse(Increment(ctx, sendCount))"}}{{end}}
{{define "Count"}}<output id="count">{{ .Result }}</output>{{end}}
```

The greeter reads client state through the reserved `signals` argument — Datastar posts its signal store as a JSON body, and muxt decodes it into the method parameter's type:

```go
func (s *Server) Greet(_ context.Context, signals GreetSignals, sendGreeting func(string) error) {
	_ = sendGreeting(cmp.Or(signals.Name, "stranger"))
}
```
