package muxt

import (
	"fmt"
	"go/ast"
)

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

// Request-body decode wrappers recognized at argument positions:
// unmarshalJSON(body) decodes the JSON request body into the method
// parameter's type; unmarshalForm(body) is the explicit spelling of the
// existing form binding.
const (
	callWrapperUnmarshalJSON = "unmarshalJSON"
	callWrapperUnmarshalForm = "unmarshalForm"
)

// isBodyUnmarshalWrapper reports whether name is a request-body decode
// wrapper pseudo-function.
func isBodyUnmarshalWrapper(name string) bool {
	return name == callWrapperUnmarshalJSON || name == callWrapperUnmarshalForm
}

// rewriteBodyFormWrappers replaces each unmarshalForm(body) argument with the
// form identifier. The two spellings are the same request.Form binding by
// construction: after this rewrite, resolution, checking, and generation all
// run the form code path.
func rewriteBodyFormWrappers(call *ast.CallExpr) {
	for i, a := range call.Args {
		nested, ok := a.(*ast.CallExpr)
		if !ok {
			continue
		}
		if fn, ok := nested.Fun.(*ast.Ident); ok && fn.Name == callWrapperUnmarshalForm {
			call.Args[i] = ast.NewIdent(TemplateNameScopeIdentifierForm)
			continue
		}
		rewriteBodyFormWrappers(nested)
	}
}

// checkBodyWrapperArguments enforces the decode-wrapper contract: exactly one
// argument, and it must be the reserved body identifier.
func checkBodyWrapperArguments(name string, call *ast.CallExpr) error {
	if len(call.Args) == 1 {
		if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == TemplateNameScopeIdentifierRequestBody {
			return nil
		}
	}
	return fmt.Errorf("the %[1]s wrapper requires exactly one argument, the reserved %[2]s identifier: %[1]s(%[2]s)", name, TemplateNameScopeIdentifierRequestBody)
}

// countBodyConsumers counts how many arguments in call (recursively) read the
// request body: the reserved body identifier and each decode wrapper. The
// request body is a single-use stream, so more than one consumer is a
// generation error.
func countBodyConsumers(call *ast.CallExpr) int {
	n := 0
	for _, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			if exp.Name == TemplateNameScopeIdentifierRequestBody {
				n++
			}
		case *ast.CallExpr:
			if fn, ok := exp.Fun.(*ast.Ident); ok && isBodyUnmarshalWrapper(fn.Name) {
				n++
				continue
			}
			n += countBodyConsumers(exp)
		}
	}
	return n
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
	for _, representation := range []Representation{RepresentationSSE, RepresentationMarshalJSON} {
		if n.fun.Name != string(representation) || len(call.Args) != 1 {
			continue
		}
		if inner, ok := call.Args[0].(*ast.CallExpr); ok {
			if innerFun, ok := inner.Fun.(*ast.Ident); ok {
				n.representation = representation
				n.call = inner
				n.fun = innerFun
			}
		}
		break
	}
	return n
}
