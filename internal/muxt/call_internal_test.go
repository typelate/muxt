package muxt

import (
	"go/ast"
	"go/parser"
	"html/template"
	"testing"
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
