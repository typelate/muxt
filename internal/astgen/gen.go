package astgen

import (
	"go/ast"
)

// ImportManager interface abstracts the import management functionality
// needed for AST generation. This allows AST generation functions to work
// with source.File without creating a circular dependency.
//
// It generally is a *muxt.File
type ImportManager interface {
	// Import registers an import and returns the package identifier to use
	Import(pkgIdent, pkgPath string) string

	// ImportSpecs returns all registered import specs
	ImportSpecs() []*ast.ImportSpec
}

// ExportedIdentifier creates a selector expression for an exported identifier
// from a package (e.g., http.ResponseWriter)
func ExportedIdentifier(im ImportManager, pkgName, pkgPath, ident string) *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent(im.Import(pkgName, pkgPath)),
		Sel: ast.NewIdent(ident),
	}
}
