package muxt

import (
	"go/ast"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/html"
)

// responseStateMethods finds the methods a package declares on its
// TemplateData type whose bodies reach .StatusCode or a .Redirect* method of
// their receiver, and names the built-in each one reaches.
//
// A method is included when its body calls one of those directly, or calls
// another method of the same type that is included; the first built-in found,
// in source order, is the one named. A method calling itself, directly or
// through others, does not reach anything by that route alone, so a cycle
// ends where it began.
//
// The analysis is syntactic: it follows calls spelled recv.Name(...) and
// recv.Header(...).Name(...) and does not resolve a receiver that has been
// shadowed. Generated methods named StatusCode and Redirect* are not
// results themselves; the template check owns the built-ins.
func responseStateMethods(files []*ast.File, templateDataType string) map[string]string {
	type method struct {
		calls []string
	}
	methods := make(map[string]method)
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
				continue
			}
			field := fn.Recv.List[0]
			if receiverTypeName(field.Type) != templateDataType || len(field.Names) != 1 {
				continue
			}
			if writesResponseState(fn.Name.Name) {
				continue
			}
			methods[fn.Name.Name] = method{calls: receiverCalls(fn.Body, field.Names[0].Name)}
		}
	}

	reached := make(map[string]string)
	for name := range methods {
		visited := map[string]bool{}
		var reach func(string) string
		reach = func(current string) string {
			if visited[current] {
				return ""
			}
			visited[current] = true
			for _, called := range methods[current].calls {
				if writesResponseState(called) {
					return called
				}
				if _, ok := methods[called]; !ok {
					continue
				}
				if builtin := reach(called); builtin != "" {
					return builtin
				}
			}
			return ""
		}
		if builtin := reach(name); builtin != "" {
			reached[name] = builtin
		}
	}
	return reached
}

// receiverTypeName is the name of the type a receiver expression declares,
// without a pointer or type parameters.
func receiverTypeName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// receiverCalls lists, in source order, the names selected from the receiver
// in body, directly or through a chain such as recv.Header(k, v).Name.
func receiverCalls(body *ast.BlockStmt, receiver string) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && rootsAtReceiver(sel.X, receiver) {
			names = append(names, sel.Sel.Name)
		}
		return true
	})
	return names
}

// rootsAtReceiver reports whether expr is the receiver or a call of a
// method chained on it.
func rootsAtReceiver(expr ast.Expr, receiver string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == receiver
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		return ok && rootsAtReceiver(sel.X, receiver)
	case *ast.ParenExpr:
		return rootsAtReceiver(e.X, receiver)
	}
	return false
}

// ResponseStatusWarnings reports, one line per route, each route whose
// method takes response and whose template calls, where dot is still the
// TemplateData, a method of the package's own that reaches .StatusCode or
// .Redirect* on its receiver. On such a route muxt writes no status code or
// redirect, so the call has no effect; unlike calling the built-in itself,
// that is not an error, because muxt cannot be sure what the method does.
//
// files are the package's syntax and templateDataType the name of the
// generated TemplateData type the methods are declared on.
func ResponseStatusWarnings(defs []Definition, files []*ast.File, templateDataType string) []string {
	var methods map[string]string
	var warnings []string
	for i := range defs {
		if !defs[i].hasResponseWriterArg || defs[i].template == nil || defs[i].template.Tree == nil {
			continue
		}
		if methods == nil {
			methods = responseStateMethods(files, templateDataType)
		}
		if len(methods) == 0 {
			return nil
		}
		name, ok := findUserResponseStateCall(defs[i].template.Tree.Root, defs[i].template, methods)
		if !ok {
			continue
		}
		remedy := "response.WriteHeader"
		if isRedirectMethod(methods[name]) {
			remedy = "http.Redirect"
		}
		warnings = append(warnings, "warning: "+defs[i].Pattern()+" calls ."+name+", which sets the status through ."+methods[name]+"; on a route that takes response the receiver method must call "+remedy+", or drop response")
	}
	return warnings
}

// findUserResponseStateCall names the first method in methods that a
// template calls on the TemplateData the handler passed in.
func findUserResponseStateCall(node template.Node, ts *html.Template, methods map[string]string) (string, bool) {
	var found string
	walkTemplateCommands(node, ts, make(map[string]bool), true, pipeIsTemplateData, func(cmd *template.CommandNode, dotIsTemplateData bool) bool {
		for _, arg := range cmd.Args {
			names := templateDataNames(arg, dotIsTemplateData)
			if len(names) == 0 {
				continue
			}
			if _, ok := methods[names[0]]; ok {
				found = names[0]
				return true
			}
		}
		return false
	})
	return found, found != ""
}
