package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

// sseEventConfigurerIdent names the unexported interface the per-event option
// type mutates; the SSE template data implements it with unexported setters.
const sseEventConfigurerIdent = "sseEventConfigurer"

// sseEventOptionDecls returns the per-event option type, its configurer
// interface, the With* constructors, and the unexported setter methods the
// SSE template data implements. Template setters run first (during
// ExecuteTemplate); call-site options apply after, so Go-side options win.
func sseEventOptionDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	stringField := func(method string) *ast.Field {
		return &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(method)},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}}},
		}
	}
	decls := []ast.Decl{
		// type SSEEventOption func(sseEventConfigurer)
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ast.NewIdent(config.SSEEventOptionType),
			Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(sseEventConfigurerIdent)}}}},
		}}},
		// type sseEventConfigurer interface { setEvent(string); setEventID(string); setRetry(int) }
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ast.NewIdent(sseEventConfigurerIdent),
			Type: &ast.InterfaceType{Methods: &ast.FieldList{List: []*ast.Field{
				stringField("setEvent"),
				stringField("setEventID"),
				{
					Names: []*ast.Ident{ast.NewIdent("setRetry")},
					Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int")}}}},
				},
			}}},
		}}},
		// func WithEvent(event string) SSEEventOption { return func(c sseEventConfigurer) { c.setEvent(event) } }
		sseEventOptionConstructor(config.SSEEventOptionType, "WithEvent", "event", ast.NewIdent("string"), "setEvent", ast.NewIdent("event")),
		sseEventOptionConstructor(config.SSEEventOptionType, "WithEventID", "id", ast.NewIdent("string"), "setEventID", ast.NewIdent("id")),
		// WithRetryDuration converts the duration to the whole milliseconds
		// the retry: wire line carries.
		sseEventOptionConstructor(config.SSEEventOptionType, "WithRetryDuration", "retryDuration",
			astgen.ExportedIdentifier(file, "", "time", "Duration"), "setRetry",
			&ast.CallExpr{Fun: ast.NewIdent("int"), Args: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ast.NewIdent("retryDuration"), Sel: ast.NewIdent("Milliseconds")},
			}}}),
		sseEventSetterMethod(config.SSETemplateDataType, "setEvent", "event", "string", sseTemplateDataFieldEvent),
		sseEventSetterMethod(config.SSETemplateDataType, "setEventID", "id", "string", sseTemplateDataFieldID),
		sseEventSetterMethod(config.SSETemplateDataType, "setRetry", "retry", "int", sseTemplateDataFieldRetry),
	}
	if config.JSONV2 {
		// WithJSONOptions forwards encoding/json/v2 and jsontext options to a
		// marshalJSON-wrapped sender's Marshal call; it exists only under
		// --output-jsonv2 and is ignored by render senders.
		jsonOptionsType := astgen.ExportedIdentifier(file, "json", "encoding/json/v2", "Options")
		iface := decls[1].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.InterfaceType)
		iface.Methods.List = append(iface.Methods.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent("setJSONOptions")},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: &ast.Ellipsis{Elt: jsonOptionsType}}}}},
		})
		decls = append(decls,
			// func WithJSONOptions(opts ...json.Options) SSEEventOption {
			//     return func(c sseEventConfigurer) { c.setJSONOptions(opts...) }
			// }
			&ast.FuncDecl{
				Name: ast.NewIdent("WithJSONOptions"),
				Type: &ast.FuncType{
					Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("jsonOptions")}, Type: &ast.Ellipsis{Elt: jsonOptionsType}}}},
					Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(config.SSEEventOptionType)}}},
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.FuncLit{
					Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
						Names: []*ast.Ident{ast.NewIdent("c")},
						Type:  ast.NewIdent(sseEventConfigurerIdent),
					}}}},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
						Fun:      &ast.SelectorExpr{X: ast.NewIdent("c"), Sel: ast.NewIdent("setJSONOptions")},
						Args:     []ast.Expr{ast.NewIdent("jsonOptions")},
						Ellipsis: 1,
					}}}},
				}}}}},
			},
			// func (data *SSETemplateData[R, T]) setJSONOptions(jsonOptions ...json.Options) { data.jsonOptions = jsonOptions }
			&ast.FuncDecl{
				Recv: sseTemplateDataMethodReceiver(config.SSETemplateDataType),
				Name: ast.NewIdent("setJSONOptions"),
				Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
					Names: []*ast.Ident{ast.NewIdent("jsonOptions")},
					Type:  &ast.Ellipsis{Elt: jsonOptionsType},
				}}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(sseTemplateDataFieldJSONOptions)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent("jsonOptions")},
				}}},
			},
		)
	}
	return decls
}

func sseEventOptionConstructor(optTypeIdent, name, paramName string, paramType ast.Expr, setter string, setterArg ast.Expr) *ast.FuncDecl {
	const cIdent = "c"
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: paramType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(optTypeIdent)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(cIdent)},
				Type:  ast.NewIdent(sseEventConfigurerIdent),
			}}}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(cIdent), Sel: ast.NewIdent(setter)},
				Args: []ast.Expr{setterArg},
			}}}},
		}}}}},
	}
}

// sseEventSetterMethod builds func (data *SSETemplateData[R, T]) setX(v string) { data.x = &v }
func sseEventSetterMethod(typeIdent, methodName, paramName, paramType, field string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(paramName)},
			Type:  ast.NewIdent(paramType),
		}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(paramName)}},
		}}},
	}
}
