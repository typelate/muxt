package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
)

func hasHTTPResponseWriterArgument(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierHTTPResponse {
				return true
			}
		case *ast.CallExpr:
			if hasHTTPResponseWriterArgument(arg) {
				return true
			}
		}
	}
	return false
}

func parseHandler(fileSet *token.FileSet, def *Definition, segments []Segment) error {
	if def.handler == "" {
		return nil
	}
	e, err := parser.ParseExprFrom(fileSet, "template_name.go", []byte(def.handler), 0)
	if err != nil {
		loc, _ := def.template.Tree.ErrorContext(def.template.Tree.Root)
		return def.spanErrorf(def.spans.call, "failed to parse handler expression %s: %v", loc, err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return errAt(e, "expected call expression, got: %s", astgen.Format(e))
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return errAt(call.Fun, "expected function identifier, got: %s", astgen.Format(call.Fun))
	}
	if representation, inner, innerFun, ok := peelRepresentationWrapper(fun, call); ok {
		def.Representation = representation
		call = inner
		fun = innerFun
	}
	if call.Ellipsis != token.NoPos {
		return errAt(call, "unexpected ellipsis")
	}

	def.usesSignals = rewriteSignalsArguments(call, segments)
	if def.Representation == RepresentationSSE {
		for _, a := range call.Args {
			if ident, ok := a.(*ast.Ident); ok && def.IsSignalsCallback(ident.Name) {
				def.signalsCallback = ident.Name
				break
			}
		}
	}

	scope := patternScope()
	for _, segment := range segments {
		if segment.IsWildcard() {
			scope = append(scope, segment.value)
		}
	}
	slices.Sort(scope)
	if err := checkArguments(scope, call, def.Representation == RepresentationSSE); err != nil {
		return err
	}
	if n := countBodyConsumers(call); n > 1 {
		return errAt(call, "call %s reads the request body %d times; the request body is a single-use stream and may be consumed at most once", astgen.Format(call.Fun), n)
	}
	rewriteBodyFormWrappers(call)

	def.fun = fun
	def.call = call

	def.hasResponseWriterArg = hasHTTPResponseWriterArgument(call)

	if (def.Representation == RepresentationSSE || def.Representation == RepresentationMarshalJSON) && def.hasResponseWriterArg {
		node := findIdent(call, TemplateNameScopeIdentifierHTTPResponse)
		if node == nil {
			node = call
		}
		return errAt(node, "%s handler cannot use a %q argument", def.Representation, TemplateNameScopeIdentifierHTTPResponse)
	}

	return nil
}

// peelRepresentationWrapper peels a representation wrapper — sse(...) or
// marshalJSON(...) — off the outermost position of a template-name call. A
// wrapper is only peeled when it has exactly one argument and that argument
// is a call to a plain identifier; otherwise the name is treated as an
// ordinary function call (a user function named "sse" keeps working).
func peelRepresentationWrapper(fun *ast.Ident, call *ast.CallExpr) (Representation, *ast.CallExpr, *ast.Ident, bool) {
	if len(call.Args) != 1 {
		return "", nil, nil, false
	}
	for _, representation := range []Representation{RepresentationSSE, RepresentationMarshalJSON} {
		if fun.Name != string(representation) {
			continue
		}
		if inner, ok := call.Args[0].(*ast.CallExpr); ok {
			if innerFun, ok := inner.Fun.(*ast.Ident); ok {
				return representation, inner, innerFun, true
			}
		}
		break
	}
	return "", nil, nil, false
}

// isSSEArgument reports whether name is an SSE render-callback argument: the
// reserved "sse" identifier, or a camelCase "sse"-prefixed name (sseClock,
// sseMetrics, ...). Prefixed callbacks render a same-named template; they are
// only valid on a route that also has the base "sse" argument.
func isSSEArgument(name string) bool {
	if name == TemplateNameScopeIdentifierExecute {
		return true
	}
	rest, ok := strings.CutPrefix(name, "sse")
	return ok && rest != "" && token.IsIdentifier(rest)
}

func checkArguments(identifiers []string, call *ast.CallExpr, sse bool) error {
	if err := checkCallArguments(identifiers, call, sse, false); err != nil {
		return err
	}
	if _, hasForm, hasMultipart := scanBodyBindings(call); hasForm && hasMultipart {
		node := findIdent(call, TemplateNameScopeIdentifierMultipart)
		if node == nil {
			node = call
		}
		return errAt(node, "call %s has both %q and %q arguments; use only one (multipart parses url-encoded fields too)", astgen.Format(call.Fun), TemplateNameScopeIdentifierForm, TemplateNameScopeIdentifierMultipart)
	}
	return nil
}

// checkCallArguments validates call's arguments against the scope. nested is
// true when call is itself an argument of the route's method call: render
// callbacks (execute and the sse callbacks) receive generated closures, and
// those are only installed for direct arguments, so a nested callback
// identifier is an error rather than generated code that does not compile.
func checkCallArguments(identifiers []string, call *ast.CallExpr, sse, nested bool) error {
	for i, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			// sse-prefixed render callbacks and Message- or Signals-suffixed
			// callbacks are only in scope on sse routes. A name already in
			// scope — a path parameter, say — keeps its scope meaning even
			// when it matches a callback naming convention.
			_, inScope := slices.BinarySearch(identifiers, exp.Name)
			sseScoped := sse && !inScope && (isSSEArgument(exp.Name) || isSSEMessageArgument(exp.Name) || isSignalsCallbackArgument(exp.Name))
			if !inScope && !sseScoped {
				if suggestion, ok := astgen.NearestString(exp.Name, identifiers); ok {
					return errAt(exp, "unknown argument %s; did you mean %s?", exp.Name, suggestion)
				}
				return errAt(exp, "unknown argument %s; expected one of: %s", exp.Name, strings.Join(identifiers, ", "))
			}
			if nested && (exp.Name == TemplateNameScopeIdentifierExecute || sseScoped) {
				return errAt(exp, "the %s callback must be a direct argument of the route's method call", exp.Name)
			}
		case *ast.CallExpr:
			if isBodyUnmarshalCall(exp) {
				if err := checkBodyWrapperArguments(exp.Fun.(*ast.Ident).Name, exp); err != nil {
					return err
				}
				continue
			}
			if err := checkCallArguments(identifiers, exp, sse, true); err != nil {
				wrapped := fmt.Errorf("call %s argument error: %w", astgen.Format(call.Fun), err)
				if pe, ok := errors.AsType[*positionedError](err); ok {
					// Keep the inner argument's position on the wrapped
					// message.
					return &positionedError{pos: pe.pos, end: pe.end, err: wrapped}
				}
				return wrapped
			}
		default:
			return errAt(a, "expected only identifier or call expressions as arguments, argument at index %d is: %s", i, astgen.Format(a))
		}
	}
	return nil
}
