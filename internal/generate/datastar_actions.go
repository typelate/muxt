package generate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	datastarActionTypeName    = "DatastarAction"
	datastarActionsTypeName   = "DatastarActions"
	datastarNewActionFuncName = "newDatastarAction"
	datastarActionsReceiver   = "routeActions"
)

// datastarActionsDecls returns the Actions() accessor, the DatastarActions type
// with one method per route, and the fixed DatastarAction fluent builder. It is
// emitted in Datastar mode.
func datastarActionsDecls(file *File, config RoutesFileConfiguration, defs []muxt.Definition) ([]ast.Decl, error) {
	support, err := datastarActionsSupportDecls(file, config, defs)
	if err != nil {
		return nil, err
	}
	decls := append([]ast.Decl{datastarActionsAccessorMethod(config.TemplateDataType)}, support...)
	return decls, nil
}

// datastarActionsSupportDecls returns the DatastarActions type, one method per
// route, and the fixed DatastarAction fluent builder — everything the Actions()
// accessor depends on, minus the accessor itself. The accessor is emitted
// separately on each render template-data type that exposes Actions().
func datastarActionsSupportDecls(file *File, config RoutesFileConfiguration, defs []muxt.Definition) ([]ast.Decl, error) {
	decls := []ast.Decl{
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(datastarActionsTypeName), Type: &ast.StructType{Fields: &ast.FieldList{
				List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")}},
			}}}},
		},
	}
	seen := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		ident, err := exportIdentifier(d.Identifier())
		if err != nil {
			return nil, err
		}
		if _, ok := seen[ident]; ok {
			continue
		}
		seen[ident] = struct{}{}
		method, err := datastarActionFunc(file, config, &d)
		if err != nil {
			return nil, err
		}
		decls = append(decls, method)
	}
	builder, err := datastarActionBuilderDecls(file)
	if err != nil {
		return nil, err
	}
	decls = append(decls, builder...)
	return decls, nil
}

// datastarActionsAccessorMethod builds the Actions() method on the template data
// type named typeName returning a DatastarActions carrying the path prefix.
func datastarActionsAccessorMethod(typeName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(typeName),
		Name: ast.NewIdent("Actions"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionsTypeName)}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CompositeLit{Type: ast.NewIdent(datastarActionsTypeName), Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(pathPrefixPathsStructFieldName)}},
			}},
		}}}},
	}
}

// datastarVerb maps an HTTP method to its Datastar backend-action function. An
// empty method (no method in the route pattern) defaults to @get.
func datastarVerb(httpMethod string) string {
	switch httpMethod {
	case "", "GET":
		return "@get"
	case "POST":
		return "@post"
	case "PUT":
		return "@put"
	case "PATCH":
		return "@patch"
	case "DELETE":
		return "@delete"
	default:
		return "@" + strings.ToLower(httpMethod)
	}
}

// datastarActionFunc builds a DatastarActions method that delegates to the
// matching TemplateRoutePaths method for the URL and wraps it in a
// DatastarAction with the route's verb. Both types share the same underlying
// struct, so the receiver converts directly.
func datastarActionFunc(file *File, config RoutesFileConfiguration, def *muxt.Definition) (*ast.FuncDecl, error) {
	ident, err := exportIdentifier(def.Identifier())
	if err != nil {
		return nil, err
	}
	fields, hasError, err := routePathParamFields(file, def)
	if err != nil {
		return nil, err
	}
	verb := datastarVerb(def.HTTPMethod())

	var args []ast.Expr
	for _, name := range def.PathValueIdentifiers() {
		args = append(args, ast.NewIdent(name))
	}
	// TemplateRoutePaths(routeActions).Ident(args...)
	pathCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.CallExpr{Fun: ast.NewIdent(config.TemplateRoutePathsTypeName), Args: []ast.Expr{ast.NewIdent(datastarActionsReceiver)}},
			Sel: ast.NewIdent(ident),
		},
		Args: args,
	}
	newAction := func(urlExpr ast.Expr) ast.Expr {
		return &ast.CallExpr{Fun: ast.NewIdent(datastarNewActionFuncName), Args: []ast.Expr{astgen.String(verb), urlExpr}}
	}

	var results *ast.FieldList
	var body []ast.Stmt
	if hasError {
		results = &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionTypeName)}, {Type: ast.NewIdent("error")}}}
		body = []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("url"), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{pathCall}},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: ast.NewIdent(datastarActionTypeName)}, ast.NewIdent(errIdent)}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{newAction(ast.NewIdent("url")), astgen.Nil()}},
		}
	} else {
		results = &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionTypeName)}}}
		body = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newAction(pathCall)}}}
	}

	return &ast.FuncDecl{
		Name: ast.NewIdent(ident),
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(datastarActionsReceiver)}, Type: ast.NewIdent(datastarActionsTypeName)}}},
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: results},
		Body: &ast.BlockStmt{List: body},
	}, nil
}

// routePathParamFields returns the method parameter fields for a route's path
// values and reports whether any path value implements encoding.TextMarshaler
// (which makes the corresponding TemplateRoutePaths method return an error). It
// mirrors the parameter grouping in routePathFunc.
func routePathParamFields(file *File, def *muxt.Definition) ([]*ast.Field, bool, error) {
	if def.Path() == "/" || def.Path() == "/{$}" {
		return nil, false, nil
	}
	encodingPkg, ok := file.Types("encoding")
	if !ok {
		return nil, false, fmt.Errorf(`the "encoding" package must be loaded`)
	}
	textMarshalerInterface := encodingPkg.Scope().Lookup("TextMarshaler").Type().Underlying().(*types.Interface)

	templatePath, _ := strings.CutSuffix(def.Path(), "{$}")
	segments := strings.Split(templatePath, "/")
	var (
		fields      []*ast.Field
		last        types.Type
		identIndex  int
		hasError    bool
		identifiers = def.PathValueIdentifiers()
	)
	for _, segment := range segments {
		if len(segment) < 1 || segment[0] != '{' || segment[len(segment)-1] != '}' {
			continue
		}
		ident := identifiers[identIndex]
		identIndex++
		pathValueType, ok := def.ArgumentType(ident)
		if !ok {
			pathValueType = types.Universe.Lookup("string").Type()
		}
		tpNode, err := file.TypeASTExpression(pathValueType)
		if err != nil {
			return nil, false, err
		}
		if last != nil && len(fields) > 0 && types.Identical(last, pathValueType) {
			fields[len(fields)-1].Names = append(fields[len(fields)-1].Names, ast.NewIdent(ident))
		} else {
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(ident)}, Type: tpNode})
			last = pathValueType
		}
		if types.Implements(pathValueType, textMarshalerInterface) {
			hasError = true
		}
	}
	return fields, hasError, nil
}

// datastarActionBuilderDecls returns the fixed DatastarAction type and its fluent
// option setters. The boilerplate is parsed from source rather than hand-built
// as AST. strconv and strings are registered so the parsed selectors resolve.
func datastarActionBuilderDecls(file *File) ([]ast.Decl, error) {
	file.Import("strconv", "strconv")
	file.Import("strings", "strings")
	file.Import("template", "html/template")

	src := "package p\n" + datastarActionBuilderSource
	parsed, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parsing datastar action builder: %w", err)
	}
	var decls []ast.Decl
	for _, d := range parsed.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		decls = append(decls, d)
	}
	return decls, nil
}

// datastarActionBuilderSource is the fixed DatastarAction fluent builder. Each
// setter returns a copy with the option appended, so an action value can be
// reused. String renders @verb('url') or @verb('url', {opt, ...}).
const datastarActionBuilderSource = `
// DatastarAction renders a Datastar backend-action expression such as
// @patch('/users/42', {openWhenHidden: true}).
type DatastarAction struct {
	verb    string
	url     string
	options []string
}

func newDatastarAction(verb, url string) DatastarAction {
	return DatastarAction{verb: verb, url: url}
}

func (a DatastarAction) with(option string) DatastarAction {
	a.options = append(append(make([]string, 0, len(a.options)+1), a.options...), option)
	return a
}

func (a DatastarAction) ContentType(contentType string) DatastarAction {
	return a.with("contentType: " + datastarJSString(contentType))
}

func (a DatastarAction) OpenWhenHidden(openWhenHidden bool) DatastarAction {
	return a.with("openWhenHidden: " + strconv.FormatBool(openWhenHidden))
}

func (a DatastarAction) Selector(selector string) DatastarAction {
	return a.with("selector: " + datastarJSString(selector))
}

func (a DatastarAction) Retry(retry string) DatastarAction {
	return a.with("retry: " + datastarJSString(retry))
}

func (a DatastarAction) RequestCancellation(requestCancellation string) DatastarAction {
	return a.with("requestCancellation: " + datastarJSString(requestCancellation))
}

func (a DatastarAction) RetryInterval(milliseconds int) DatastarAction {
	return a.with("retryInterval: " + strconv.Itoa(milliseconds))
}

func (a DatastarAction) RetryScaler(scaler float64) DatastarAction {
	return a.with("retryScaler: " + strconv.FormatFloat(scaler, 'g', -1, 64))
}

func (a DatastarAction) RetryMaxWait(milliseconds int) DatastarAction {
	return a.with("retryMaxWait: " + strconv.Itoa(milliseconds))
}

func (a DatastarAction) RetryMaxCount(count int) DatastarAction {
	return a.with("retryMaxCount: " + strconv.Itoa(count))
}

func (a DatastarAction) String() string {
	if len(a.options) == 0 {
		return a.verb + "('" + a.url + "')"
	}
	return a.verb + "('" + a.url + "', {" + strings.Join(a.options, ", ") + "})"
}

// JS returns the action as template.JS so it renders verbatim inside Datastar
// data-on-* event attributes, which html/template parses as a JavaScript
// context. Without it, the expression would be JSON-wrapped and escaped.
func (a DatastarAction) JS() template.JS {
	return template.JS(a.String())
}

// datastarJSString renders a single-quoted JavaScript string literal.
func datastarJSString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '\'':
			b.WriteString("\\'")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}
`
