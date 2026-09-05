package muxt

import (
	"go/ast"
	"go/parser"
	"html/template"
	"testing"

	"github.com/typelate/muxt/internal/astgen"
)

func mustParseCall(t *testing.T, src string) *ast.CallExpr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("ParseExpr(%q) = %v", src, err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		t.Fatalf("ParseExpr(%q) is %T, want *ast.CallExpr", src, e)
	}
	return call
}

func TestCountBodyConsumers(t *testing.T) {
	for _, tt := range []struct {
		expr string
		want int
	}{
		{expr: `Save(ctx)`, want: 0},
		{expr: `Save(ctx, body)`, want: 1},
		{expr: `Save(ctx, unmarshalJSON(body))`, want: 1},
		{expr: `Save(ctx, body, unmarshalJSON(body))`, want: 2},
		{expr: `Save(ctx, unmarshalJSON(body), unmarshalJSON(body))`, want: 2},
		{expr: `Outer(Inner(ctx, body))`, want: 1},
		{expr: `Save(ctx, unmarshalForm(body))`, want: 1},
		{expr: `Save(ctx, unmarshalForm(body), unmarshalJSON(body))`, want: 2},
		{expr: `Save(ctx, form)`, want: 1},
		{expr: `Save(ctx, form, form)`, want: 1},
		{expr: `Save(ctx, form, unmarshalForm(body))`, want: 1},
		{expr: `Save(ctx, form, body)`, want: 2},
		{expr: `Save(ctx, multipart, unmarshalJSON(body))`, want: 2},
		{expr: `Outer(Inner(form), body)`, want: 2},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			if got := countBodyConsumers(mustParseCall(t, tt.expr)); got != tt.want {
				t.Errorf("countBodyConsumers(%q) = %d, want %d", tt.expr, got, tt.want)
			}
		})
	}
}

func TestDefinitionsBodyArgumentErrors(t *testing.T) {
	for _, tt := range []struct {
		name, template, wantErr string
	}{
		{name: "unmarshalJSON requires the body identifier", template: `{{define "POST / Save(unmarshalJSON(form))"}}{{end}}`, wantErr: "the unmarshalJSON wrapper requires exactly one argument, the reserved body identifier: unmarshalJSON(body)"},
		{name: "unmarshalJSON requires exactly one argument", template: `{{define "POST / Save(unmarshalJSON(body, ctx))"}}{{end}}`, wantErr: "the unmarshalJSON wrapper requires exactly one argument, the reserved body identifier: unmarshalJSON(body)"},
		{name: "request body may be consumed at most once", template: `{{define "POST / Save(ctx, body, unmarshalJSON(body))"}}{{end}}`, wantErr: "call Save reads the request body 2 times; the request body is a single-use stream and may be consumed at most once"},
		{name: "unmarshalForm requires the body identifier", template: `{{define "POST / Save(unmarshalForm(form))"}}{{end}}`, wantErr: "the unmarshalForm wrapper requires exactly one argument, the reserved body identifier: unmarshalForm(body)"},
		{name: "form parses the request body", template: `{{define "POST / Save(ctx, form, body)"}}{{end}}`, wantErr: "call Save reads the request body 2 times; the request body is a single-use stream and may be consumed at most once"},
		{name: "multipart parses the request body", template: `{{define "POST / Save(ctx, multipart, unmarshalJSON(body))"}}{{end}}`, wantErr: "call Save reads the request body 2 times; the request body is a single-use stream and may be consumed at most once"},
		{name: "execute nested in a call argument", template: `{{define "GET / Outer(Inner(execute))"}}{{end}}`, wantErr: "call Outer argument error: the execute callback must be a direct argument of the route's method call"},
		{name: "execute nested inside a representation wrapper call argument", template: `{{define "GET / marshalJSON(Outer(Inner(execute)))"}}{{end}}`, wantErr: "call Outer argument error: the execute callback must be a direct argument of the route's method call"},
		{name: "sse callback nested in a call argument", template: `{{define "GET / sse(Outer(Inner(sseClock)))"}}{{end}}`, wantErr: "call Outer argument error: the sseClock callback must be a direct argument of the route's method call"},
		{name: "unmarshalForm conflicts with multipart like form does", template: `{{define "POST / Save(unmarshalForm(body), multipart)"}}{{end}}`, wantErr: `call Save has both "form" and "multipart" arguments; use only one (multipart parses url-encoded fields too)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(tt.template))
			_, err := Definitions(ts, "templates", nil)
			if err == nil {
				t.Fatalf("Definitions(%q) = nil error, want %q", tt.template, tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Definitions(%q) error = %q, want %q", tt.template, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRewriteBodyFormWrappers(t *testing.T) {
	for _, tt := range []struct {
		expr, want string
	}{
		{expr: `Save(ctx, unmarshalForm(body))`, want: `Save(ctx, form)`},
		{expr: `Save(ctx, unmarshalJSON(body))`, want: `Save(ctx, unmarshalJSON(body))`},
		{expr: `Outer(Inner(unmarshalForm(body)))`, want: `Outer(Inner(form))`},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			call := mustParseCall(t, tt.expr)
			rewriteBodyFormWrappers(call)
			if got := astgen.Format(call); got != tt.want {
				t.Errorf("rewriteBodyFormWrappers(%q) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestPeelRepresentationWrapper(t *testing.T) {
	for _, tt := range []struct {
		expr           string
		representation Representation
		fun            string
		peeled         bool
	}{
		{expr: `sse(Stream(ctx, execute))`, representation: RepresentationSSE, fun: "Stream", peeled: true},
		{expr: `marshalJSON(List(ctx))`, representation: RepresentationMarshalJSON, fun: "List", peeled: true},
		{expr: `List(ctx)`, peeled: false},
		{expr: `marshalJSON()`, peeled: false},
		{expr: `marshalJSON(ctx)`, peeled: false},
		{expr: `marshalJSON(A(ctx), B(ctx))`, peeled: false},
		{expr: `marshalJSON(pkg.Fn(ctx))`, peeled: false},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			call := mustParseCall(t, tt.expr)
			representation, inner, innerFun, ok := peelRepresentationWrapper(call.Fun.(*ast.Ident), call)
			if ok != tt.peeled {
				t.Fatalf("peelRepresentationWrapper(%q) ok = %t, want %t", tt.expr, ok, tt.peeled)
			}
			if !tt.peeled {
				return
			}
			if representation != tt.representation {
				t.Errorf("peelRepresentationWrapper(%q) representation = %q, want %q", tt.expr, representation, tt.representation)
			}
			if innerFun.Name != tt.fun {
				t.Errorf("peelRepresentationWrapper(%q) fun = %q, want %q", tt.expr, innerFun.Name, tt.fun)
			}
			if inner == nil {
				t.Errorf("peelRepresentationWrapper(%q) inner call is nil", tt.expr)
			}
		})
	}
}

func TestRewriteSignalsArguments(t *testing.T) {
	for _, tt := range []struct {
		expr           string
		pathValueNames []string
		want           string
		rewritten      bool
	}{
		{expr: `Save(ctx, signals)`, want: `Save(ctx, unmarshalJSON(body))`, rewritten: true},
		{expr: `sse(Search(ctx, signals, sseResults))`, want: `sse(Search(ctx, unmarshalJSON(body), sseResults))`, rewritten: true},
		{expr: `Save(ctx, form)`, want: `Save(ctx, form)`},
		{expr: `Show(ctx, signals)`, pathValueNames: []string{"signals"}, want: `Show(ctx, signals)`},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			call := mustParseCall(t, tt.expr)
			rewritten := rewriteSignalsArguments(call, tt.pathValueNames)
			if rewritten != tt.rewritten {
				t.Errorf("rewriteSignalsArguments(%q) = %t, want %t", tt.expr, rewritten, tt.rewritten)
			}
			if got := astgen.Format(call); got != tt.want {
				t.Errorf("rewriteSignalsArguments(%q) rewrote to %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestDefinitionsSignals(t *testing.T) {
	t.Run("signals marks the definition", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "POST /search Save(ctx, signals)"}}{{end}}`))
		defs, err := Definitions(ts, "templates", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !defs[0].UsesSignals() {
			t.Error("UsesSignals() = false, want true")
		}
		if got, want := astgen.Format(defs[0].CallExpression()), "Save(ctx, unmarshalJSON(body))"; got != want {
			t.Errorf("call = %q, want %q", got, want)
		}
	})
	t.Run("a signals path wildcard keeps its path-value meaning", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET /s/{signals} Show(ctx, signals)"}}{{end}}`))
		defs, err := Definitions(ts, "templates", nil)
		if err != nil {
			t.Fatal(err)
		}
		if defs[0].UsesSignals() {
			t.Error("UsesSignals() = true, want false")
		}
	})
}

func TestIsSignalsCallbackArgument(t *testing.T) {
	for name, want := range map[string]bool{
		"countsSignals":     true,
		"Signals":           true,
		"signals":           false,
		"countsSignal":      false,
		"signalsCounts":     false,
		"boardStateSignals": true,
	} {
		if got := IsSignalsCallbackArgument(name); got != want {
			t.Errorf("IsSignalsCallbackArgument(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestDefinitionsSignalsCallback(t *testing.T) {
	ts := template.Must(template.New("").Parse(`{{define "GET /board sse(Stream(ctx, execute, countsSignals))"}}{{end}}`))
	defs, err := Definitions(ts, "templates", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := defs[0].SignalsCallback()
	if !ok || name != "countsSignals" {
		t.Errorf("SignalsCallback() = %q, %t; want %q, true", name, ok, "countsSignals")
	}
}
