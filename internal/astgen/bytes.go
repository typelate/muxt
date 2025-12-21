package astgen

import (
	"go/ast"
)

// BytesNewBuffer creates a bytes.NewBuffer call expression
func BytesNewBuffer(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(im.Import("", "bytes")),
			Sel: ast.NewIdent("NewBuffer"),
		},
		Args: []ast.Expr{expr},
	}
}
