package astgen

import (
	"go/ast"
	"go/types"
)

// ImportManager interface abstracts the import management functionality
// needed for AST generation. This allows AST generation functions to work
// with source.File without creating a circular dependency.
type ImportManager interface {
	// Import registers an import and returns the package identifier to use
	Import(pkgIdent, pkgPath string) string

	// ImportSpecs returns all registered import specs
	ImportSpecs() []*ast.ImportSpec

	// TypeASTExpression converts a types.Type to an AST expression
	TypeASTExpression(tp types.Type) (ast.Expr, error)

	// Types looks up a types.Package by path
	Types(pkgPath string) (*types.Package, bool)
}

// ExportedIdentifier creates a selector expression for an exported identifier
// from a package (e.g., http.ResponseWriter)
func ExportedIdentifier(im ImportManager, pkgName, pkgPath, ident string) *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent(im.Import(pkgName, pkgPath)),
		Sel: ast.NewIdent(ident),
	}
}

// Call creates a function call expression for a package function
func Call(im ImportManager, pkgName, pkgPath, funcIdent string, args []ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, pkgName, pkgPath, funcIdent),
		Args: args,
	}
}
