package muxt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

func mustParseCall(t *testing.T, src string) *ast.CallExpr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	require.NoError(t, err, "ParseExpr(%q)", src)
	call, ok := e.(*ast.CallExpr)
	require.True(t, ok, "ParseExpr(%q) is %T, want *ast.CallExpr", src, e)
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
			assert.Equal(t, tt.want, countBodyConsumers(mustParseCall(t, tt.expr)), "countBodyConsumers(%q)", tt.expr)
		})
	}
}

func TestDefinitionsBodyArgumentErrors(t *testing.T) {
	const (
		unmarshalJSONShape = "the unmarshalJSON wrapper requires exactly one argument, the reserved body identifier: unmarshalJSON(body)"
		unmarshalFormShape = "the unmarshalForm wrapper requires exactly one argument, the reserved body identifier: unmarshalForm(body)"
		consumedTwice      = "call Save reads the request body 2 times; the request body is a single-use stream and may be consumed at most once"
		nestedExecute      = "call Outer argument error: the execute callback must be a direct argument of the route's method call"
	)
	for _, tt := range []struct {
		name, template, wantErr string
	}{
		{name: "unmarshalJSON requires the body identifier", template: `{{define "POST / Save(unmarshalJSON(form))"}}{{end}}`, wantErr: unmarshalJSONShape},
		{name: "unmarshalJSON requires exactly one argument", template: `{{define "POST / Save(unmarshalJSON(body, ctx))"}}{{end}}`, wantErr: unmarshalJSONShape},
		{name: "request body may be consumed at most once", template: `{{define "POST / Save(ctx, body, unmarshalJSON(body))"}}{{end}}`, wantErr: consumedTwice},
		{name: "unmarshalForm requires the body identifier", template: `{{define "POST / Save(unmarshalForm(form))"}}{{end}}`, wantErr: unmarshalFormShape},
		{name: "form parses the request body", template: `{{define "POST / Save(ctx, form, body)"}}{{end}}`, wantErr: consumedTwice},
		{name: "multipart parses the request body", template: `{{define "POST / Save(ctx, multipart, unmarshalJSON(body))"}}{{end}}`, wantErr: consumedTwice},
		{name: "execute nested in a call argument", template: `{{define "GET / Outer(Inner(execute))"}}{{end}}`, wantErr: nestedExecute},
		{name: "execute nested inside a representation wrapper call argument", template: `{{define "GET / marshalJSON(Outer(Inner(execute)))"}}{{end}}`, wantErr: nestedExecute},
		{name: "sse callback nested in a call argument", template: `{{define "GET / sse(Outer(Inner(sseClock)))"}}{{end}}`, wantErr: "call Outer argument error: the sseClock callback must be a direct argument of the route's method call"},
		{name: "unmarshalForm conflicts with multipart like form does", template: `{{define "POST / Save(unmarshalForm(body), multipart)"}}{{end}}`, wantErr: `call Save has both "form" and "multipart" arguments; use only one (multipart parses url-encoded fields too)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(tt.template))
			_, err := Definitions(source.Variable{Name: "templates", Set: ts})
			require.Error(t, err, "Definitions(%q)", tt.template)
			assert.Equal(t, tt.wantErr, err.Error(), "Definitions(%q) error", tt.template)
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
			assert.Equal(t, tt.want, astgen.Format(call), "rewriteBodyFormWrappers(%q)", tt.expr)
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
			require.Equal(t, tt.peeled, ok, "peelRepresentationWrapper(%q) ok", tt.expr)
			if !tt.peeled {
				return
			}
			assert.Equal(t, tt.representation, representation, "peelRepresentationWrapper(%q) representation", tt.expr)
			assert.Equal(t, tt.fun, innerFun.Name, "peelRepresentationWrapper(%q) fun", tt.expr)
			assert.NotNil(t, inner, "peelRepresentationWrapper(%q) inner call", tt.expr)
		})
	}
}

func TestRewriteSignalsArguments(t *testing.T) {
	for _, tt := range []struct {
		expr      string
		segments  []Segment
		want      string
		rewritten bool
	}{
		{expr: `Save(ctx, signals)`, want: `Save(ctx, unmarshalJSON(body))`, rewritten: true},
		{expr: `sse(Search(ctx, signals, sseResults))`, want: `sse(Search(ctx, unmarshalJSON(body), sseResults))`, rewritten: true},
		{expr: `Save(ctx, form)`, want: `Save(ctx, form)`},
		{expr: `Show(ctx, signals)`, segments: []Segment{newSegment("{signals}")}, want: `Show(ctx, signals)`},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			call := mustParseCall(t, tt.expr)
			rewritten := rewriteSignalsArguments(call, tt.segments)
			assert.Equal(t, tt.rewritten, rewritten, "rewriteSignalsArguments(%q)", tt.expr)
			assert.Equal(t, tt.want, astgen.Format(call), "rewriteSignalsArguments(%q) rewrote to", tt.expr)
		})
	}
}

func TestDefinitionsSignals(t *testing.T) {
	t.Run("signals marks the definition", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "POST /search Save(ctx, signals)"}}{{end}}`))
		defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
		require.NoError(t, err)
		assert.True(t, defs[0].UsesSignals(), "UsesSignals()")
		assert.Equal(t, "Save(ctx, unmarshalJSON(body))", astgen.Format(defs[0].CallExpression()), "call")
	})
	t.Run("a signals path wildcard keeps its path-value meaning", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET /s/{signals} Show(ctx, signals)"}}{{end}}`))
		defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
		require.NoError(t, err)
		assert.False(t, defs[0].UsesSignals(), "UsesSignals()")
	})
}

func TestIsSignalsCallbackArgument(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{name: "countsSignals", want: true},
		{name: "Signals", want: true},
		{name: "signals", want: false},
		{name: "countsSignal", want: false},
		{name: "signalsCounts", want: false},
		{name: "boardStateSignals", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSignalsCallbackArgument(tt.name), "isSignalsCallbackArgument(%q)", tt.name)
		})
	}
}

func TestDefinitionsSignalsCallback(t *testing.T) {
	ts := template.Must(template.New("").Parse(`{{define "GET /board sse(Stream(ctx, execute, countsSignals))"}}{{end}}`))
	defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
	require.NoError(t, err)
	name, ok := defs[0].SignalsCallback()
	assert.True(t, ok, "SignalsCallback() ok")
	assert.Equal(t, "countsSignals", name, "SignalsCallback() name")
}

// TestTypeQualifier states how a type is named in a message about a route:
// a type the receiver's own package declares is named on its own, and one
// from anywhere else carries its package name.
func TestTypeQualifier(t *testing.T) {
	receiverPkg := types.NewPackage("example.com/server", "server")
	otherPkg := types.NewPackage("example.com/other/models", "models")
	qual := typeQualifier(receiverPkg)

	named := func(pkg *types.Package, name string) types.Type {
		obj := types.NewTypeName(token.NoPos, pkg, name, nil)
		return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	}
	assert.Equal(t, "Page", types.TypeString(named(receiverPkg, "Page"), qual), "a receiver package type")
	assert.Equal(t, "models.Page", types.TypeString(named(otherPkg, "Page"), qual), "another package's type")
}
