package generate

import (
	"go/ast"

	"github.com/typelate/muxt/internal/astgen"
)

// datastarTemplateDataDecls returns the DatastarTemplateData render type named
// typeName and its methods. It is the text/html render type for routes wrapped
// in datastar(...): the base template-data type plus an Actions() accessor and
// the datastar response-header setters (datastar-selector / datastar-mode /
// datastar-use-view-transition). The header setters are chainable (they return
// the receiver pointer) so they render as "" via the type's String() method.
func datastarTemplateDataDecls(file *File, config RoutesFileConfiguration, typeName string) []ast.Decl {
	decls := templateDataDecls(file, config, typeName, false)
	decls = append(decls,
		datastarActionsAccessorMethod(typeName),
		datastarRenderHeaderSetterMethod(typeName, "Selector", "datastar-selector", "selector"),
		datastarRenderHeaderSetterMethod(typeName, "Mode", "datastar-mode", "mode"),
		datastarRenderUseViewTransitionMethod(file, typeName),
	)
	return decls
}

// datastarRenderHeaderSetterMethod builds a chainable string-arg setter that
// sets headerName on the response via the base Header method and returns the
// receiver pointer. It mirrors htmxHeaderSetterMethod but is named neutrally so
// the datastar feature does not couple to an htmx-named helper.
func datastarRenderHeaderSetterMethod(templateDataTypeIdent, methodName, headerName, paramName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(paramName)},
				Type:  ast.NewIdent("string"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexListExpr{
					X:       ast.NewIdent(templateDataTypeIdent),
					Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.CallExpr{
						Fun:  &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("Header")},
						Args: []ast.Expr{astgen.String(headerName), ast.NewIdent(paramName)},
					}},
				},
			},
		},
	}
}

// datastarRenderUseViewTransitionMethod builds a chainable bool-arg setter that
// sets the datastar-use-view-transition response header to the string form of
// the bool (strconv.FormatBool, matching the action builder's bool stringify)
// and returns the receiver pointer.
func datastarRenderUseViewTransitionMethod(file *File, templateDataTypeIdent string) *ast.FuncDecl {
	const paramName = "useViewTransition"
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("UseViewTransition"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(paramName)},
				Type:  ast.NewIdent("bool"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexListExpr{
					X:       ast.NewIdent(templateDataTypeIdent),
					Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("Header")},
						Args: []ast.Expr{
							astgen.String("datastar-use-view-transition"),
							astgen.Call(file, "strconv", "strconv", "FormatBool", ast.NewIdent(paramName)),
						},
					}},
				},
			},
		},
	}
}
