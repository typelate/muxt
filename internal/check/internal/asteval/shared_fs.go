package asteval

import (
	"fmt"
	"go/ast"
	"go/types"
)

// sharedFiles resolves arg, a pkg.Var selector, to the absolute paths of the
// files the other package's //go:embed on Var matched, the way embedFSFilePaths
// does for the analyzed package's own variable. It returns the import path of
// the package that owns them.
//
// Only a package of the analyzed package's module is read: its files are
// part of the same build, so the embed patterns the muxt user wrote are the
// ones the go command resolved. Anything else is an error, one plain error
// for now.
func sharedFiles(scope Scope, arg *ast.SelectorExpr) (paths []string, importPath string, err error) {
	pkgIdent, ok := arg.X.(*ast.Ident)
	if !ok {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("first argument to ParseFS must be an identifier or an exported variable of an imported package"))
	}
	pkgName, ok := scope.Info.Uses[pkgIdent].(*types.PkgName)
	if !ok {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("first argument to ParseFS must be an identifier or an exported variable of an imported package"))
	}
	importPath = pkgName.Imported().Path()
	other, ok := scope.Imports[importPath]
	if !ok || other.ModulePath != scope.ModulePath {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("package %s is not in this module (%s); a shared embed.FS must come from a package of this module", importPath, scope.ModulePath))
	}
	if !arg.Sel.IsExported() {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("%s.%s is unexported; ParseFS reads only an exported embed.FS variable of another package", pkgIdent.Name, arg.Sel.Name))
	}
	relative, err := RelativeFilePaths(other.Dir, other.EmbedFiles...)
	if err != nil {
		return nil, "", fmt.Errorf("calculating relative paths of files embedded by %s: %w", importPath, err)
	}
	matches, err := embedFSFilePaths(scope.Dir, scope.Fset, other.Vars, arg.Sel, relative)
	if err != nil {
		return nil, "", err
	}
	return joinFilePaths(other.Dir, matches...), importPath, nil
}
