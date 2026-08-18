package muxt

import (
	"go/ast"
	"go/parser"
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

func TestNormalizeCall(t *testing.T) {
	for _, tt := range []struct {
		name           string
		expr           string
		framing        Framing
		representation Representation
		fun            string
		argCount       int
	}{
		{name: "plain method call", expr: `List(ctx)`, framing: FramingNone, representation: "", fun: "List", argCount: 1},
		{name: "sse wraps a call", expr: `sse(Stream(ctx, lastEventID, execute))`, framing: FramingNone, representation: RepresentationSSE, fun: "Stream", argCount: 3},
		{name: "sse zero args is a user function call", expr: `sse()`, framing: FramingNone, representation: "", fun: "sse", argCount: 0},
		{name: "sse with non-call arg is a user function call", expr: `sse(ctx)`, framing: FramingNone, representation: "", fun: "sse", argCount: 1},
		{name: "sse with two args is a user function call", expr: `sse(A(ctx), B(ctx))`, framing: FramingNone, representation: "", fun: "sse", argCount: 2},
		{name: "sse around selector-fun call is a user function call", expr: `sse(pkg.Fn(ctx))`, framing: FramingNone, representation: "", fun: "sse", argCount: 1},
		{name: "htmx wraps a call", expr: `htmx(Save(ctx, form))`, framing: FramingHTMX, representation: "", fun: "Save", argCount: 2},
		{name: "htmx wraps a representation", expr: `htmx(sse(Stream(ctx, execute)))`, framing: FramingHTMX, representation: RepresentationSSE, fun: "Stream", argCount: 2},
		{name: "datastar wraps a call", expr: `datastar(Save(ctx, form))`, framing: FramingDatastar, representation: "", fun: "Save", argCount: 2},
		{name: "datastar wraps a representation", expr: `datastar(sse(Stream(ctx, execute)))`, framing: FramingDatastar, representation: RepresentationSSE, fun: "Stream", argCount: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			n, err := normalizeCall(mustParseCall(t, tt.expr))
			if err != nil {
				t.Fatalf("normalizeCall(%q) = %v", tt.expr, err)
			}
			if n.framing != tt.framing {
				t.Errorf("normalizeCall(%q).framing = %q, want %q", tt.expr, n.framing, tt.framing)
			}
			if n.representation != tt.representation {
				t.Errorf("normalizeCall(%q).representation = %q, want %q", tt.expr, n.representation, tt.representation)
			}
			if n.fun.Name != tt.fun {
				t.Errorf("normalizeCall(%q).fun = %q, want %q", tt.expr, n.fun.Name, tt.fun)
			}
			if got := len(n.call.Args); got != tt.argCount {
				t.Errorf("normalizeCall(%q) arg count = %d, want %d", tt.expr, got, tt.argCount)
			}
		})
	}
}

func TestNormalizeCallFramingErrors(t *testing.T) {
	for _, tt := range []struct {
		name, expr, wantErr string
	}{
		{name: "htmx no args", expr: `htmx()`, wantErr: "the htmx framing wrapper takes exactly one method call argument, got 0 arguments"},
		{name: "htmx two args", expr: `htmx(A(ctx), B(ctx))`, wantErr: "the htmx framing wrapper takes exactly one method call argument, got 2 arguments"},
		{name: "htmx non-call arg", expr: `htmx(ctx)`, wantErr: "the htmx framing wrapper takes exactly one method call argument, got ctx"},
		{name: "datastar no args", expr: `datastar()`, wantErr: "the datastar framing wrapper takes exactly one method call argument, got 0 arguments"},
		{name: "datastar selector fun", expr: `datastar(pkg.Fn(ctx))`, wantErr: "expected function identifier, got: pkg.Fn"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeCall(mustParseCall(t, tt.expr))
			if err == nil {
				t.Fatalf("normalizeCall(%q) = nil error, want %q", tt.expr, tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("normalizeCall(%q) error = %q, want %q", tt.expr, err.Error(), tt.wantErr)
			}
		})
	}
}
