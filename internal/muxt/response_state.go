package muxt

import (
	"fmt"
	"html/template"
	"strings"
	"text/template/parse"
)

// checkResponseWriterConflicts rejects a template that asks muxt to set the
// response status when the receiver method has taken the response.
//
// A method with an http.ResponseWriter argument owns the response, so muxt
// generates no status code and no redirect for that route. A template calling
// .Redirect, one of its siblings, or .StatusCode is then writing a field of
// TemplateData that nothing reads: the template parses, the handler compiles,
// and at run time the redirect simply does not happen. Saying so here is the
// whole point -- the alternative is a route that looks right and silently is
// not.
func checkResponseWriterConflicts(ts *template.Template, defs []Definition) error {
	var errs []error
	for i := range defs {
		if !defs[i].hasResponseWriterArg {
			continue
		}
		t := ts.Lookup(defs[i].name)
		if t == nil || t.Tree == nil {
			continue
		}
		method, found := findResponseStateCall(t.Tree.Root, ts)
		if !found {
			continue
		}
		function := defs[i].handler
		if defs[i].fun != nil {
			function = defs[i].fun.Name
		}
		errs = append(errs, &ResponseWriterTemplateStateError{
			Location: defs[i].definitionLocation(),
			Template: defs[i].name,
			Method:   method,
			Function: function,
		})
	}
	return CombineErrors(errs)
}

// ResponseWriterTemplateStateError reports a template that sets response state
// muxt will not write, because the route's method took the response.
type ResponseWriterTemplateStateError struct {
	// Location is where the template name was written.
	Location string

	// Template is the full template name, Method the TemplateData method the
	// template calls, and Function the receiver method that took the response.
	Template, Method, Function string
}

func (e *ResponseWriterTemplateStateError) Error() string {
	var sb strings.Builder
	if e.Location != "" {
		sb.WriteString(e.Location)
		sb.WriteString(": ")
	}
	remedy := "call response.WriteHeader in the method"
	if isRedirectMethod(e.Method) {
		remedy = "call http.Redirect in the method"
	}
	_, _ = fmt.Fprintf(&sb, "template %q calls %s but %s takes the http.ResponseWriter, so muxt writes no status code or redirect for this route: either drop the response argument or %s",
		e.Template, e.Method, e.Function, remedy)
	return sb.String()
}

// findResponseStateCall reports the first TemplateData method a template calls
// that only has an effect through the response muxt would write.
func findResponseStateCall(node parse.Node, ts *template.Template) (string, bool) {
	var found string
	walkTemplateCommands(node, ts, make(map[string]bool), true, func(cmd *parse.CommandNode, dotIsTemplateData bool) bool {
		method, ok := responseStateCallInCommand(cmd, dotIsTemplateData)
		if ok {
			found = method
		}
		return ok
	})
	return found, found != ""
}

// responseStateCallInCommand names the method when a command calls one that
// only takes effect through the response muxt writes.
func responseStateCallInCommand(cmd *parse.CommandNode, dotIsTemplateData bool) (string, bool) {
	if cmd == nil {
		return "", false
	}
	for _, arg := range cmd.Args {
		switch a := arg.(type) {
		case *parse.FieldNode:
			// .StatusCode is only TemplateData's when dot still is.
			// Inside a with or a range it names a field of whatever was
			// selected, and reporting that would reject a working route.
			if !dotIsTemplateData {
				continue
			}
			for _, ident := range a.Ident {
				if writesResponseState(ident) {
					return ident, true
				}
			}
		case *parse.VariableNode:
			// $ is the dot the template started with, whatever the
			// current one is, so $.StatusCode is TemplateData's wherever
			// it is written.
			if len(a.Ident) == 0 || a.Ident[0] != "$" {
				continue
			}
			for _, ident := range a.Ident[1:] {
				if writesResponseState(ident) {
					return ident, true
				}
			}
		case *parse.ChainNode:
			if !chainStartsAtTemplateData(a, dotIsTemplateData) {
				continue
			}
			for _, field := range a.Field {
				if writesResponseState(field) {
					return field, true
				}
			}
		}
	}
	return "", false
}

// chainStartsAtTemplateData reports whether a chained expression is rooted
// at the TemplateData the handler passed in.
func chainStartsAtTemplateData(chain *parse.ChainNode, dotIsTemplateData bool) bool {
	switch node := chain.Node.(type) {
	case *parse.DotNode:
		return dotIsTemplateData
	case *parse.VariableNode:
		return len(node.Ident) > 0 && node.Ident[0] == "$"
	case *parse.PipeNode:
		// (.Redirect "/x").Header and the like: the parenthesised
		// pipeline is walked on its own, so the chain only has to say
		// whether its own fields are TemplateData's.
		return dotIsTemplateData
	default:
		return false
	}
}

// writesResponseState reports whether a TemplateData method records something
// that reaches the client only through the status line muxt writes.
//
// .Header is not one of them: it writes to the response header map directly,
// so it works whoever owns the response.
func writesResponseState(methodName string) bool {
	return isRedirectMethod(methodName) || methodName == "StatusCode"
}
