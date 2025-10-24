package astgen

import "go/ast"

func ErrorsNew(im ImportManager, in ast.Expr) *ast.CallExpr {
	return Call(im, "", "errors", "New", in)
}

func ErrorsJoin(im ImportManager, in ...ast.Expr) *ast.CallExpr {
	return Call(im, "", "errors", "Join", in...)
}
