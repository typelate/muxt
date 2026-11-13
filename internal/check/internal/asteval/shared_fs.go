package asteval

import (
	"fmt"
	"go/ast"
	"go/types"

	"github.com/typelate/muxt/internal/check/internal/astgen"
)

// sharedFiles resolves arg, a pkg.Var selector, to the files the other
// package's //go:embed on Var matched, the way embedFSFilePaths does for the
// analyzed package's own variable. The paths are relative to the other
// package's directory, which the scope's Imports holds under the import path
// it also returns.
//
// Only a package of the analyzed package's module is read: its files are
// part of the same build, so the embed patterns the muxt user wrote are the
// ones the go command resolved. A package of another module is an
// OtherModuleError; one the scope holds no embedded files for, which is what
// a scope with nil Imports (check.Package) always means, is a
// SharedPackageUnavailableError.
func sharedFiles(scope Scope, arg *ast.SelectorExpr) (paths []string, importPath string, err error) {
	pkgIdent, ok := arg.X.(*ast.Ident)
	if !ok {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("%s (got %s)", firstArgumentToParseFSMessage, astgen.Format(arg)))
	}
	pkgName, ok := scope.Info.Uses[pkgIdent].(*types.PkgName)
	if !ok {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("%s (got %s)", firstArgumentToParseFSMessage, astgen.Format(arg)))
	}
	importPath = pkgName.Imported().Path()
	other, ok := scope.Imports[importPath]
	if !ok {
		return nil, "", SharedPackageUnavailableError{Position: scope.Fset.Position(arg.Pos()), ImportPath: importPath}
	}
	if other.ModulePath != scope.ModulePath {
		return nil, "", OtherModuleError{Position: scope.Fset.Position(arg.Pos()), ImportPath: importPath, OtherModule: other.ModulePath, ThisModule: scope.ModulePath}
	}
	if !arg.Sel.IsExported() {
		return nil, "", UnexportedSharedFSError{Position: scope.Fset.Position(arg.Pos()), Package: pkgIdent.Name, Name: arg.Sel.Name}
	}
	// The analyzed package's type information types the selector; a var
	// that is not an embed.FS has no embedded files this function can read.
	v, typed := scope.Info.Uses[arg.Sel].(*types.Var)
	if typed && !isEmbedFS(v.Type()) {
		return nil, "", NotEmbedFSError{Position: scope.Fset.Position(arg.Pos()), Package: pkgIdent.Name, Name: arg.Sel.Name, Type: types.TypeString(v.Type(), nil)}
	}
	// A package that embeds no files has no //go:embed directive, since a
	// pattern that matches nothing fails the build, so its files are not
	// parsed: a syntax error in an import ParseFS never names stays the
	// compiler's to report.
	_, patterns, found := embedDirective(other.Vars, arg.Sel.Name)
	if (typed && len(other.EmbedFiles) == 0) || (found && len(patterns) == 0) {
		return nil, "", NoEmbedDirectiveError{Position: scope.Fset.Position(arg.Pos()), Package: pkgIdent.Name, Name: arg.Sel.Name}
	}
	relative, err := RelativeFilePaths(other.Dir, other.EmbedFiles...)
	if err != nil {
		return nil, "", fmt.Errorf("calculating relative paths of files embedded by %s: %w", importPath, err)
	}
	matches, err := embedFSFilePaths(scope.Dir, scope.Fset, other.Vars, arg.Sel, relative)
	if err != nil {
		return nil, "", err
	}
	return matches, importPath, nil
}

// isEmbedFS reports whether t is the embed package's FS type.
func isEmbedFS(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "embed" && named.Obj().Name() == "FS"
}
