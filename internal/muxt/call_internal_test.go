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
		{name: "unmarshalForm conflicts with multipart like form does", template: `{{define "POST / Save(unmarshalForm(body), multipart)"}}{{end}}`, wantErr: `call Save has both "form" and "multipart" arguments; use only one (multipart parses url-encoded fields too)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(tt.template))
			_, err := Definitions(ts, "templates")
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
