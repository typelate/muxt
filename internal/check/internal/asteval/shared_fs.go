package asteval

import (
	"fmt"
	"go/ast"
	"go/types"

	"github.com/typelate/muxt/internal/check/internal/astgen"
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
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("%s (got %s)", firstArgumentToParseFSMessage, astgen.Format(arg)))
	}
	pkgName, ok := scope.Info.Uses[pkgIdent].(*types.PkgName)
	if !ok {
		return nil, "", wrapWithFilename(scope.Dir, scope.Fset, arg.Pos(), fmt.Errorf("%s (got %s)", firstArgumentToParseFSMessage, astgen.Format(arg)))
	}
	importPath = pkgName.Imported().Path()
	other, ok := scope.Imports[importPath]
	if !ok || other.ModulePath != scope.ModulePath {
		return nil, "", OtherModuleError{Position: relativePosition(scope.Dir, scope.Fset, arg.Pos()), ImportPath: importPath, ThisModule: scope.ModulePath}
	}
	if !arg.Sel.IsExported() {
		return nil, "", UnexportedSharedFSError{Position: relativePosition(scope.Dir, scope.Fset, arg.Pos()), Package: pkgIdent.Name, Name: arg.Sel.Name}
	}
	// The analyzed package's type information types the selector; a var
	// that is not an embed.FS has no embedded files this function can read.
	if v, ok := scope.Info.Uses[arg.Sel].(*types.Var); ok && !isEmbedFS(v.Type()) {
		return nil, "", NotEmbedFSError{Position: relativePosition(scope.Dir, scope.Fset, arg.Pos()), Package: pkgIdent.Name, Name: arg.Sel.Name, Type: types.TypeString(v.Type(), nil)}
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

// isEmbedFS reports whether t is the embed package's FS type.
func isEmbedFS(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "embed" && named.Obj().Name() == "FS"
}
