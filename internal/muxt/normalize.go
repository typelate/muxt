package muxt

import "go/ast"

// Framing identifies the frontend wrapper at the outermost position of a
// template-name call expression: frame( representation( Method(args…) ) ).
// No framings are recognized yet; the field exists so wrapper peeling,
// validation, and code generation can branch on one explicit model instead
// of re-inspecting the AST.
type Framing string

const FramingNone Framing = ""

// normalizedCall is the explicit model a template-name call expression
// reduces to before any validation or code generation.
type normalizedCall struct {
	framing        Framing
	representation Representation
	fun            *ast.Ident
	call           *ast.CallExpr
}

// normalizeCall peels recognized wrapper pseudo-functions off call and
// records them. A wrapper is only peeled when it has exactly one argument
// and that argument is a call to a plain identifier; otherwise the name is
// treated as an ordinary function call (a user function named "sse" keeps
// working).
func normalizeCall(call *ast.CallExpr) normalizedCall {
	n := normalizedCall{
		framing: FramingNone,
		fun:     call.Fun.(*ast.Ident),
		call:    call,
	}
	if n.fun.Name == string(RepresentationSSE) && len(call.Args) == 1 {
		if inner, ok := call.Args[0].(*ast.CallExpr); ok {
			if innerFun, ok := inner.Fun.(*ast.Ident); ok {
				n.representation = RepresentationSSE
				n.call = inner
				n.fun = innerFun
			}
		}
	}
	return n
}
