package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

// sseEventConfigurerIdent names the unexported interface the per-event option
// type mutates; the SSE template data implements it with unexported setters.
const sseEventConfigurerIdent = "sseEventConfigurer"

// sseEventOptionConfig selects which parts of the per-event option surface a
// generated file needs: which event template-data types implement the
// configurer interface and whether the Datastar vocabulary is present.
type sseEventOptionConfig struct {
	genericEventTD  bool
	datastarEventTD bool
}

// sseEventOptionDecls returns the per-event option types, the shared
// configurer interface, the With* constructors, and the unexported setter
// methods the event template-data types implement. All option types share the
// configurer-function underlying type and every constructor returns the
// unnamed func(sseEventConfigurer), so a shared constructor (WithEventID,
// WithRetryDuration, WithJSONOptions) assigns to each named option variadic.
// Template setters run first (during ExecuteTemplate); call-site options
// apply after, so Go-side options win.
func sseEventOptionDecls(file *File, config RoutesFileConfiguration, oc sseEventOptionConfig) []ast.Decl {
	stringField := func(method string) *ast.Field {
		return &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(method)},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}}},
		}
	}
	boolField := func(method string) *ast.Field {
		return &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(method)},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}}},
		}
	}
	optionType := func(name string) ast.Decl {
		return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ast.NewIdent(name),
			Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(sseEventConfigurerIdent)}}}},
		}}}
	}
	ifaceFields := []*ast.Field{
		stringField("setEvent"),
		stringField("setEventID"),
		{
			Names: []*ast.Ident{ast.NewIdent("setRetry")},
			Type:  &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int")}}}},
		},
	}
	if oc.datastarEventTD {
		ifaceFields = append(ifaceFields,
			stringField("setSelector"),
			stringField("setMode"),
			stringField("setNamespace"),
			boolField("setUseViewTransition"),
			boolField("setOnlyIfMissing"),
		)
	}
	decls := []ast.Decl{
		optionType(config.SSEEventOptionType),
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ast.NewIdent(sseEventConfigurerIdent),
			Type: &ast.InterfaceType{Methods: &ast.FieldList{List: ifaceFields}},
		}}},
		// func WithEvent(event string) func(sseEventConfigurer) { return func(c sseEventConfigurer) { c.setEvent(event) } }
		sseEventOptionConstructor("WithEvent", "event", ast.NewIdent("string"), "setEvent", ast.NewIdent("event")),
		sseEventOptionConstructor("WithEventID", "id", ast.NewIdent("string"), "setEventID", ast.NewIdent("id")),
		// WithRetryDuration converts the duration to the whole milliseconds
		// the retry: wire line carries.
		sseEventOptionConstructor("WithRetryDuration", "retryDuration",
			astgen.ExportedIdentifier(file, "", "time", "Duration"), "setRetry",
			&ast.CallExpr{Fun: ast.NewIdent("int"), Args: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ast.NewIdent("retryDuration"), Sel: ast.NewIdent("Milliseconds")},
			}}}),
	}
	if oc.genericEventTD {
		decls = append(decls,
			sseEventSetterMethod(config.SSETemplateDataType, "setEvent", "event", "string", sseTemplateDataFieldEvent),
			sseEventSetterMethod(config.SSETemplateDataType, "setEventID", "id", "string", sseTemplateDataFieldID),
			sseEventSetterMethod(config.SSETemplateDataType, "setRetry", "retry", "int", sseTemplateDataFieldRetry),
		)
		if oc.datastarEventTD {
			// The Datastar setters are meaningless on the generic event type;
			// no-op implementations keep it satisfying the shared configurer.
			decls = append(decls,
				sseEventNoopSetterMethod(config.SSETemplateDataType, "setSelector", "string"),
				sseEventNoopSetterMethod(config.SSETemplateDataType, "setMode", "string"),
				sseEventNoopSetterMethod(config.SSETemplateDataType, "setNamespace", "string"),
				sseEventNoopSetterMethod(config.SSETemplateDataType, "setUseViewTransition", "bool"),
				sseEventNoopSetterMethod(config.SSETemplateDataType, "setOnlyIfMissing", "bool"),
			)
		}
	}
	if oc.datastarEventTD {
		decls = append(decls,
			optionType(config.DatastarPatchElementOptionType()),
			optionType(config.DatastarPatchSignalsOptionType()),
			sseEventOptionConstructor("WithSelector", "selector", ast.NewIdent("string"), "setSelector", ast.NewIdent("selector")),
			// WithSelectorID targets an element by id attribute.
			sseEventOptionConstructor("WithSelectorID", "id", ast.NewIdent("string"), "setSelector",
				&ast.BinaryExpr{X: astgen.String("#"), Op: token.ADD, Y: ast.NewIdent("id")}),
			sseEventOptionConstructor("WithMode", "mode", ast.NewIdent("string"), "setMode", ast.NewIdent("mode")),
			sseEventOptionConstructor("WithNamespace", "namespace", ast.NewIdent("string"), "setNamespace", ast.NewIdent("namespace")),
			sseEventOptionConstructor("WithUseViewTransition", "useViewTransition", ast.NewIdent("bool"), "setUseViewTransition", ast.NewIdent("useViewTransition")),
			sseEventOptionConstructor("WithOnlyIfMissing", "onlyIfMissing", ast.NewIdent("bool"), "setOnlyIfMissing", ast.NewIdent("onlyIfMissing")),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setEvent", "event", "string", sseTemplateDataFieldEvent),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setEventID", "id", "string", sseTemplateDataFieldID),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setRetry", "retry", "int", sseTemplateDataFieldRetry),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setSelector", "selector", "string", datastarEventFieldSelector),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setMode", "mode", "string", datastarEventFieldMode),
			sseEventSetterMethod(config.DatastarEventTemplateDataType, "setNamespace", "namespace", "string", datastarEventFieldNamespace),
			sseEventBoolSetterMethod(config.DatastarEventTemplateDataType, "setUseViewTransition", datastarEventFieldUseViewTransition),
			sseEventBoolSetterMethod(config.DatastarEventTemplateDataType, "setOnlyIfMissing", datastarEventFieldOnlyIfMissing),
		)
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
		setJSONOptionsMethod := func(typeIdent string) *ast.FuncDecl {
			return &ast.FuncDecl{
				Recv: sseTemplateDataMethodReceiver(typeIdent),
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
			}
		}
		decls = append(decls,
			// func WithJSONOptions(opts ...json.Options) func(sseEventConfigurer) {
			//     return func(c sseEventConfigurer) { c.setJSONOptions(opts...) }
			// }
			&ast.FuncDecl{
				Name: ast.NewIdent("WithJSONOptions"),
				Type: &ast.FuncType{
					Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("jsonOptions")}, Type: &ast.Ellipsis{Elt: jsonOptionsType}}}},
					Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(sseEventConfigurerIdent)}}}}}}},
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
		)
		if oc.genericEventTD {
			decls = append(decls, setJSONOptionsMethod(config.SSETemplateDataType))
		}
		if oc.datastarEventTD {
			decls = append(decls, setJSONOptionsMethod(config.DatastarEventTemplateDataType))
		}
	}
	return decls
}

func sseEventOptionConstructor(name, paramName string, paramType ast.Expr, setter string, setterArg ast.Expr) *ast.FuncDecl {
	const cIdent = "c"
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: paramType}}},
			// The unnamed configurer-function return type assigns to every
			// named option type, so shared constructors serve them all.
			Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(sseEventConfigurerIdent)}}}}}}},
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

// sseEventBoolSetterMethod builds func (data *T[R, T]) setX(v bool) { data.x = v }
func sseEventBoolSetterMethod(typeIdent, methodName, field string) *ast.FuncDecl {
	const paramName = "value"
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(paramName)},
			Type:  ast.NewIdent("bool"),
		}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(paramName)},
		}}},
	}
}

// sseEventNoopSetterMethod builds an empty-bodied setter so a template-data
// type satisfies configurer methods that do not apply to it.
func sseEventNoopSetterMethod(typeIdent, methodName, paramType string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{
			Type: ast.NewIdent(paramType),
		}}}},
		Body: &ast.BlockStmt{},
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
