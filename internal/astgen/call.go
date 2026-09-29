package astgen

import "go/ast"

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
