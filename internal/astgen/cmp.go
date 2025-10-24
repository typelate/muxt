package astgen

import "go/ast"

func CmpOr(im ImportManager, in ...ast.Expr) *ast.CallExpr {
	return Call(im, "", "cmp", "Or", in...)
}
