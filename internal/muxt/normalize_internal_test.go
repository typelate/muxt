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
	} {
		t.Run(tt.name, func(t *testing.T) {
			n := normalizeCall(mustParseCall(t, tt.expr))
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
