package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

const (
	datastarReceiverName = "action"
	datastarActionName   = "Action"
	datastarOpenField    = "openWhenHidden"
)

// datastarStringOption is a fluent setter of the datastar type for an option
// whose value is a string: the method, the field it sets, the name of its
// parameter and the key of the option.
type datastarStringOption struct{ method, field, param, key string }

var datastarStringOptions = []datastarStringOption{
	{"ContentType", "contentType", "kind", "contentType"},
	{"Selector", "selector", "selector", "selector"},
	{"RequestCancellation", "requestCancellation", "mode", "requestCancellation"},
}

func routeDatastarTypeName(config RoutesFileConfiguration) string {
	return routeTypeName(config) + "Datastar"
}

// routeDatastarDecls emits the type TemplateRoute.Datastar returns, which
// builds the action of a route:
//
//	data-on:click="{{((.Path.Create).Datastar.ContentType "form").Action}}"
//
// The verb comes from the route's method and its argument is the path. Action
// returns a template.JS because html/template writes a template.JS into a
// data-on:* attribute as the JavaScript it is.
func routeDatastarDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	typeName := routeDatastarTypeName(config)
	typeIdent := ast.NewIdent(typeName)
	fields := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("route")}, Type: ast.NewIdent(routeTypeName(config))}}
	for _, option := range datastarStringOptions {
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(option.field)}, Type: ast.NewIdent("string")})
	}
	fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(datastarOpenField)}, Type: &ast.StarExpr{X: ast.NewIdent("bool")}})
	decls := []ast.Decl{
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
			&ast.TypeSpec{Name: typeIdent, Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}},
		}},
		&ast.FuncDecl{
			Name: ast.NewIdent("Datastar"),
			Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(routeTypeName(config)), routeReceiverName)}},
			Type: &ast.FuncType{Params: &ast.FieldList{}, Results: fieldList(results(typeIdent))},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(&ast.CompositeLit{
				Type: typeIdent,
				Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("route"), Value: ast.NewIdent(routeReceiverName)}},
			})}},
		},
	}
	for _, option := range datastarStringOptions {
		decls = append(decls, datastarSetter(typeIdent, option.method, option.field, option.param, ast.NewIdent("string"), ast.NewIdent(option.param)))
	}
	decls = append(decls, datastarSetter(typeIdent, "OpenWhenHidden", datastarOpenField, "open", ast.NewIdent("bool"),
		&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent("open")}))
	return append(decls, datastarActionMethod(file, typeIdent))
}

// datastarSetter emits a method that returns a copy of the receiver with one field set.
func datastarSetter(typeIdent *ast.Ident, method, field, paramName string, paramType, value ast.Expr) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent(method),
		Recv: &ast.FieldList{List: []*ast.Field{param(typeIdent, datastarReceiverName)}},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{param(paramType, paramName)}},
			Results: fieldList(results(typeIdent)),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{selector(datastarReceiverName, field)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{value},
			},
			returnExprs(ast.NewIdent(datastarReceiverName)),
		}},
	}
}

// datastarActionMethod emits:
//
//	func (action TemplateRouteDatastar) Action() (template.JS, error) {
//		verb := strings.ToLower(action.route.method)
//		if verb == "" {
//			return "", fmt.Errorf("route %s has no HTTP method to build a datastar action from", action.route.path)
//		}
//		path, err := json.Marshal(action.route.path)
//		if err != nil {
//			return "", fmt.Errorf("failed to marshal the path of route %s: %w", action.route.path, err)
//		}
//		options := map[string]any{}
//		if action.contentType != "" {
//			options["contentType"] = action.contentType
//		}
//		...
//		if action.openWhenHidden != nil {
//			options["openWhenHidden"] = *action.openWhenHidden
//		}
//		call := "@" + verb + "(" + string(path)
//		if len(options) > 0 {
//			encoded, err := json.Marshal(options)
//			if err != nil {
//				return "", fmt.Errorf("failed to marshal the options of route %s: %w", action.route.path, err)
//			}
//			call += ", " + string(encoded)
//		}
//		return template.JS(call + ")"), nil
//	}
//
// The options are JSON because a JSON object is a JavaScript object literal and
// json.Marshal escapes the strings in it.
func datastarActionMethod(file *File, typeIdent *ast.Ident) *ast.FuncDecl {
	const (
		verbIdent    = "verb"
		pathIdent    = "path"
		optionsIdent = "options"
		callIdent    = "call"
		encodedIdent = "encoded"
	)
	routeSelector := func(field string) ast.Expr {
		return &ast.SelectorExpr{X: selector(datastarReceiverName, "route"), Sel: ast.NewIdent(field)}
	}
	marshalOrReturn := func(valueIdent string, value ast.Expr, what string) []ast.Stmt {
		return []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(valueIdent), ast.NewIdent(errIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{astgen.Call(file, "json", "encoding/json", "Marshal", value)},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(
					astgen.String(""),
					astgen.Call(file, "fmt", "fmt", "Errorf", astgen.String("failed to marshal the "+what+" of route %s: %w"), routeSelector("path"), ast.NewIdent(errIdent)),
				)}},
			},
		}
	}
	setOption := func(key string, value ast.Expr) ast.Stmt {
		return &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(optionsIdent), Index: astgen.String(key)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value},
		}
	}
	notEqual := func(x, y ast.Expr) ast.Expr { return &ast.BinaryExpr{X: x, Op: token.NEQ, Y: y} }
	concat := func(x, y ast.Expr) ast.Expr { return &ast.BinaryExpr{X: x, Op: token.ADD, Y: y} }

	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(verbIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.Call(file, "strings", "strings", "ToLower", routeSelector("method"))},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(verbIdent), Op: token.EQL, Y: astgen.String("")},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(
				astgen.String(""),
				astgen.Call(file, "fmt", "fmt", "Errorf", astgen.String("route %s has no HTTP method to build a datastar action from"), routeSelector("path")),
			)}},
		},
	}
	body = append(body, marshalOrReturn(pathIdent, routeSelector("path"), "path")...)
	body = append(body, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(optionsIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CompositeLit{Type: &ast.MapType{Key: ast.NewIdent("string"), Value: ast.NewIdent("any")}}},
	})
	for _, option := range datastarStringOptions {
		field := selector(datastarReceiverName, option.field)
		body = append(body, &ast.IfStmt{
			Cond: notEqual(field, astgen.String("")),
			Body: &ast.BlockStmt{List: []ast.Stmt{setOption(option.key, field)}},
		})
	}
	openField := selector(datastarReceiverName, datastarOpenField)
	body = append(body,
		&ast.IfStmt{
			Cond: notEqual(openField, astgen.Nil()),
			Body: &ast.BlockStmt{List: []ast.Stmt{setOption(datastarOpenField, &ast.StarExpr{X: openField})}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(callIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{concat(concat(concat(astgen.String("@"), ast.NewIdent(verbIdent)), astgen.String("(")), astgen.ConvertIdent("string", ast.NewIdent(pathIdent)))},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: astgen.CallBuiltinLen(ast.NewIdent(optionsIdent)), Op: token.GTR, Y: astgen.Int(0)},
			Body: &ast.BlockStmt{List: append(
				marshalOrReturn(encodedIdent, ast.NewIdent(optionsIdent), "options"),
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(callIdent)},
					Tok: token.ADD_ASSIGN,
					Rhs: []ast.Expr{concat(astgen.String(", "), astgen.ConvertIdent("string", ast.NewIdent(encodedIdent)))},
				},
			)},
		},
		returnExprs(
			&ast.CallExpr{
				Fun:  astgen.ExportedIdentifier(file, "template", "html/template", "JS"),
				Args: []ast.Expr{concat(ast.NewIdent(callIdent), astgen.String(")"))},
			},
			astgen.Nil(),
		),
	)
	return &ast.FuncDecl{
		Name: ast.NewIdent(datastarActionName),
		Recv: &ast.FieldList{List: []*ast.Field{param(typeIdent, datastarReceiverName)}},
		Type: &ast.FuncType{
			Params: &ast.FieldList{},
			Results: fieldList(results(
				astgen.ExportedIdentifier(file, "template", "html/template", "JS"),
				ast.NewIdent("error"),
			)),
		},
		Body: &ast.BlockStmt{List: body},
	}
}
