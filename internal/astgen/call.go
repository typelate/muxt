package astgen

import "go/ast"

// Call creates a function call expression for a package function
func Call(im ImportManager, pkgName, pkgPath, funcIdent string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: ExportedIdentifier(im, pkgName, pkgPath, funcIdent), Args: args}
}

func CallBuiltin(funcIdent string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: ast.NewIdent(funcIdent), Args: args}
}

func CallBuiltinLen(args ast.Expr) *ast.CallExpr { return CallBuiltin("len", args) }

func CallBuiltinAppend(slice ast.Expr, in ...ast.Expr) *ast.CallExpr {
	return CallBuiltin("append", append([]ast.Expr{slice}, in...)...)
}

func Convert(tp ast.Expr, expr ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: tp, Args: []ast.Expr{expr}}
}

func ConvertIdent(tp string, expr ast.Expr) *ast.CallExpr {
	return Convert(ast.NewIdent(tp), expr)
}

func ResultsWithErr(typ ast.Expr) *ast.FieldList {
	return &ast.FieldList{
		List: []*ast.Field{
			{Type: typ},
			{Type: ast.NewIdent("error")},
		},
	}
}
