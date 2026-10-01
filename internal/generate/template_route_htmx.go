package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

const (
	htmxReceiverName   = "attrs"
	htmxAttributesName = "Attributes"
)

// htmxAttribute is a fluent setter of the htmx type: the method, the field it
// sets and the name of its parameter.
type htmxAttribute struct{ method, field, param, name string }

// htmxAttributes are the attributes after the verb attribute, in the order they
// render.
var htmxAttributes = []htmxAttribute{
	{"Target", "target", "selector", "hx-target"},
	{"Swap", "swap", "strategy", "hx-swap"},
	{"Trigger", "trigger", "event", "hx-trigger"},
	{"PushURL", "pushURL", "value", "hx-push-url"},
}

func routeHTMXTypeName(config RoutesFileConfiguration) string {
	return routeTypeName(config) + "HTMX"
}

// routeHTMXDecls emits the type TemplateRoute.HTMX returns, which builds the
// hx-* attributes of a route:
//
//	{{((.Path.Delete 7).HTMX.Target "#row").Attributes}}
//
// The verb attribute comes from the route's method and its value is the path.
// Attributes returns a template.HTMLAttr because html/template only writes
// attributes into a tag when the value says it is one.
func routeHTMXDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	typeName := routeHTMXTypeName(config)
	typeIdent := ast.NewIdent(typeName)
	fields := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("route")}, Type: ast.NewIdent(routeTypeName(config))}}
	for _, attribute := range htmxAttributes {
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(attribute.field)}, Type: ast.NewIdent("string")})
	}
	decls := []ast.Decl{
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
			&ast.TypeSpec{Name: typeIdent, Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}},
		}},
		&ast.FuncDecl{
			Name: ast.NewIdent("HTMX"),
			Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(routeTypeName(config)), routeReceiverName)}},
			Type: &ast.FuncType{Params: &ast.FieldList{}, Results: fieldList(results(typeIdent))},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(&ast.CompositeLit{
				Type: typeIdent,
				Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent("route"), Value: ast.NewIdent(routeReceiverName)}},
			})}},
		},
	}
	for _, attribute := range htmxAttributes {
		decls = append(decls, htmxSetter(typeIdent, attribute))
	}
	return append(decls, htmxAttributesMethod(file, config, typeIdent))
}

// htmxSetter emits a method that returns a copy of the receiver with one field set.
func htmxSetter(typeIdent *ast.Ident, attribute htmxAttribute) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent(attribute.method),
		Recv: &ast.FieldList{List: []*ast.Field{param(typeIdent, htmxReceiverName)}},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{param(ast.NewIdent("string"), attribute.param)}},
			Results: fieldList(results(typeIdent)),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{selector(htmxReceiverName, attribute.field)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ast.NewIdent(attribute.param)},
			},
			returnExprs(ast.NewIdent(htmxReceiverName)),
		}},
	}
}

// htmxAttributesMethod emits:
//
//	func (attrs TemplateRouteHTMX) Attributes() (template.HTMLAttr, error) {
//		verb := strings.ToLower(attrs.route.method)
//		if verb == "" {
//			return "", fmt.Errorf("route %s has no HTTP method to build an htmx attribute from", attrs.route.path)
//		}
//		pairs := [][2]string{{"hx-" + verb, attrs.route.path}, {"hx-target", attrs.target}, ...}
//		var attributes []string
//		for _, pair := range pairs {
//			if pair[1] != "" {
//				attributes = append(attributes, pair[0]+`="`+html.EscapeString(pair[1])+`"`)
//			}
//		}
//		return template.HTMLAttr(strings.Join(attributes, " ")), nil
//	}
func htmxAttributesMethod(file *File, config RoutesFileConfiguration, typeIdent *ast.Ident) *ast.FuncDecl {
	const (
		verbIdent       = "verb"
		pairsIdent      = "pairs"
		attributesIdent = "attributes"
		pairIdent       = "pair"
	)
	routeSelector := func(field string) ast.Expr {
		return &ast.SelectorExpr{X: selector(htmxReceiverName, "route"), Sel: ast.NewIdent(field)}
	}
	pairElement := func(name, value ast.Expr) ast.Expr {
		return &ast.CompositeLit{Elts: []ast.Expr{name, value}}
	}
	pairElements := []ast.Expr{pairElement(
		&ast.BinaryExpr{X: astgen.String("hx-"), Op: token.ADD, Y: ast.NewIdent(verbIdent)},
		routeSelector("path"),
	)}
	for _, attribute := range htmxAttributes {
		pairElements = append(pairElements, pairElement(astgen.String(attribute.name), selector(htmxReceiverName, attribute.field)))
	}
	pairIndex := func(i int) ast.Expr {
		return &ast.IndexExpr{X: ast.NewIdent(pairIdent), Index: astgen.Int(i)}
	}
	quoted := &ast.BinaryExpr{
		X: &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: pairIndex(0), Op: token.ADD, Y: astgen.String(`="`)},
			Op: token.ADD,
			Y:  astgen.Call(file, "html", "html", "EscapeString", pairIndex(1)),
		},
		Op: token.ADD,
		Y:  astgen.String(`"`),
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(htmxAttributesName),
		Recv: &ast.FieldList{List: []*ast.Field{param(typeIdent, htmxReceiverName)}},
		Type: &ast.FuncType{
			Params: &ast.FieldList{},
			Results: fieldList(results(
				astgen.ExportedIdentifier(file, "template", "html/template", "HTMLAttr"),
				ast.NewIdent("error"),
			)),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(verbIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{astgen.Call(file, "strings", "strings", "ToLower", routeSelector("method"))},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(verbIdent), Op: token.EQL, Y: astgen.String("")},
				Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(
					astgen.String(""),
					astgen.Call(file, "fmt", "fmt", "Errorf", astgen.String("route %s has no HTTP method to build an htmx attribute from"), routeSelector("path")),
				)}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(pairsIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CompositeLit{
					Type: &ast.ArrayType{Elt: &ast.ArrayType{Len: astgen.Int(2), Elt: ast.NewIdent("string")}},
					Elts: pairElements,
				}},
			},
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
				&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(attributesIdent)}, Type: &ast.ArrayType{Elt: ast.NewIdent("string")}},
			}}},
			&ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent(pairIdent),
				Tok:   token.DEFINE,
				X:     ast.NewIdent(pairsIdent),
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: pairIndex(1), Op: token.NEQ, Y: astgen.String("")},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(attributesIdent)},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{astgen.CallBuiltinAppend(ast.NewIdent(attributesIdent), quoted)},
					}}},
				}}},
			},
			returnExprs(
				&ast.CallExpr{
					Fun:  astgen.ExportedIdentifier(file, "template", "html/template", "HTMLAttr"),
					Args: []ast.Expr{astgen.Call(file, "strings", "strings", "Join", ast.NewIdent(attributesIdent), astgen.String(" "))},
				},
				astgen.Nil(),
			),
		}},
	}
}
