package astgen

import (
	"go/ast"
)

// TimeParseCall creates a time.Parse call expression
func TimeParseCall(im ImportManager, layout string, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "time", "Parse", String(layout), expr)
}
