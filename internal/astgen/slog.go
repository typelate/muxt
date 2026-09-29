package astgen

import (
	"go/ast"
)

func SlogLoggerPtr(im ImportManager) *ast.StarExpr {
	return &ast.StarExpr{X: ExportedIdentifier(im, "", "log/slog", "Logger")}
}

func SlogString(im ImportManager, key string, val ast.Expr) *ast.CallExpr {
	return Call(im, "", "log/slog", "String", String(key), val)
}
