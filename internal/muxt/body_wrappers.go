package muxt

import (
	"go/ast"
)

// Request-body decode wrappers recognized at argument positions:
// unmarshalJSON(body) decodes the JSON request body into the method
// parameter's type; unmarshalForm(body) is the explicit spelling of the
// existing form binding.
const (
	callWrapperUnmarshalJSON = "unmarshalJSON"
	callWrapperUnmarshalForm = "unmarshalForm"
)

// isBodyUnmarshalCall reports whether call invokes one of the request-body
// decode wrappers.
func isBodyUnmarshalCall(call *ast.CallExpr) bool {
	return isCallTo(call, callWrapperUnmarshalJSON) || isCallTo(call, callWrapperUnmarshalForm)
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
		if isCallTo(nested, callWrapperUnmarshalForm) {
			// The identifier starts where the wrapper was written, so
			// an error about it marks the wrapper.
			call.Args[i] = &ast.Ident{NamePos: nested.Pos(), Name: TemplateNameScopeIdentifierForm}
			continue
		}
		rewriteBodyFormWrappers(nested)
	}
}

// isCallTo reports whether call invokes the plain identifier name.
func isCallTo(call *ast.CallExpr, name string) bool {
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == name
}

// checkBodyWrapperArguments enforces the decode-wrapper contract: exactly one
// argument, and it must be the reserved body identifier.
func checkBodyWrapperArguments(name string, call *ast.CallExpr) error {
	if len(call.Args) == 1 {
		if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == TemplateNameScopeIdentifierRequestBody {
			return nil
		}
		// Point at the one wrong argument rather than the whole wrapper.
		return errAt(call.Args[0], "the %[1]s wrapper requires exactly one argument, the reserved %[2]s identifier: %[1]s(%[2]s)", name, TemplateNameScopeIdentifierRequestBody)
	}
	return errAt(call, "the %[1]s wrapper requires exactly one argument, the reserved %[2]s identifier: %[1]s(%[2]s)", name, TemplateNameScopeIdentifierRequestBody)
}

// rewriteSignalsArguments replaces each signals argument with
// unmarshalJSON(body): Datastar sends the page's signal state as the JSON
// request body, so the sugar and the explicit spelling bind identically. A
// path wildcard named signals keeps its path-value meaning. It reports
// whether anything was rewritten.
func rewriteSignalsArguments(call *ast.CallExpr, segments []Segment) bool {
	if hasPathParameter(segments, TemplateNameScopeIdentifierSignals) {
		return false
	}
	rewritten := false
	for i, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierSignals {
				// The call spans what was written, so an error about
				// it marks signals.
				call.Args[i] = &ast.CallExpr{
					Fun:    &ast.Ident{NamePos: arg.Pos(), Name: callWrapperUnmarshalJSON},
					Lparen: arg.Pos(),
					Args:   []ast.Expr{&ast.Ident{NamePos: arg.Pos(), Name: TemplateNameScopeIdentifierRequestBody}},
					Rparen: arg.End() - 1,
				}
				rewritten = true
			}
		case *ast.CallExpr:
			if rewriteSignalsArguments(arg, segments) {
				rewritten = true
			}
		}
	}
	return rewritten
}

// countBodyConsumers counts how many times call (recursively) reads the
// request body. Each body identifier and each unmarshalJSON(body) wrapper
// reads the stream directly. The form and multipart bindings — including
// unmarshalForm(body), which is the form binding — parse it through
// request.ParseForm / request.ParseMultipartForm, which cache the result, so
// however many times they appear they count as one read. The request body is
// a single-use stream, so more than one read is a generation error.
func countBodyConsumers(call *ast.CallExpr) int {
	b := scanBodyBindings(call)
	if b.hasForm || b.hasMultipart {
		b.reads++
	}
	return b.reads
}

type bodyBindings struct {
	reads                 int
	hasForm, hasMultipart bool
}

// scanBodyBindings walks call's argument tree, counting direct request-body
// reads (the body identifier and the unmarshalJSON(body) wrapper) and noting
// the form and multipart bindings (the form and multipart identifiers, and
// unmarshalForm(body), which is the form binding).
func scanBodyBindings(call *ast.CallExpr) bodyBindings {
	var b bodyBindings
	for _, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			b.addIdent(exp.Name)
		case *ast.CallExpr:
			b.addCall(exp)
		}
	}
	return b
}

func (b *bodyBindings) addIdent(name string) {
	switch name {
	case TemplateNameScopeIdentifierRequestBody:
		b.reads++
	case TemplateNameScopeIdentifierForm:
		b.hasForm = true
	case TemplateNameScopeIdentifierMultipart:
		b.hasMultipart = true
	}
}

func (b *bodyBindings) addCall(exp *ast.CallExpr) {
	switch {
	case isCallTo(exp, callWrapperUnmarshalJSON):
		b.reads++
	case isCallTo(exp, callWrapperUnmarshalForm):
		b.hasForm = true
	default:
		nested := scanBodyBindings(exp)
		b.reads += nested.reads
		b.hasForm = b.hasForm || nested.hasForm
		b.hasMultipart = b.hasMultipart || nested.hasMultipart
	}
}
