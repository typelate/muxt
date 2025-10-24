package astgen

import (
	"go/ast"
)

// SlogString creates a slog.String call expression
func SlogString(im ImportManager, key string, val ast.Expr) *ast.CallExpr {
	return Call(im, "", "log/slog", "String", []ast.Expr{String(key), val})
}