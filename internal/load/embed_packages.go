package load

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/source"
)

// embedPackages describes each direct import of pkg that a ParseFS call
// could name, by import path. It reads go/packages values already in pkg's
// load and returns plain data, so reading another package's embedded files
// costs no second load.
//
// An import with no module (the standard library) is described with an empty
// ModulePath. It and an import in another module, or one that embeds
// nothing, are returned with Vars nil: they are described, so the evaluator
// can say why they cannot be read, but their files are not parsed. A package
// of this module that embeds files has its Go files parsed with comments into
// pkg's token.FileSet, because the //go:embed comment that decides which
// files a variable holds is on the declaration, and the load does not keep
// the syntax of an import.
func embedPackages(pkg *packages.Package) (map[string]source.EmbedPackage, error) {
	result := make(map[string]source.EmbedPackage)
	// Sorted, so the first error is the same on every run.
	for _, path := range slices.Sorted(maps.Keys(pkg.Imports)) {
		imported := pkg.Imports[path]
		ep := source.EmbedPackage{
			ImportPath: imported.PkgPath,
			EmbedFiles: slices.Sorted(slices.Values(imported.EmbedFiles)),
		}
		if len(imported.GoFiles) > 0 {
			ep.Dir = filepath.Dir(imported.GoFiles[0])
		}
		if imported.Module != nil {
			ep.ModulePath = imported.Module.Path
		}
		if pkg.Module != nil && imported.Module != nil && imported.Module.Path == pkg.Module.Path && len(imported.EmbedFiles) > 0 {
			vars, err := varDeclarations(pkg.Fset, imported.GoFiles)
			if err != nil {
				return nil, fmt.Errorf("reading package %s: %w", path, err)
			}
			ep.Vars = vars
		}
		result[path] = ep
	}
	return result, nil
}

// varDeclarations parses files with their comments into fset and returns the
// package-level var declarations of all of them.
func varDeclarations(fset *token.FileSet, files []string) ([]*ast.GenDecl, error) {
	var decls []*ast.GenDecl
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				decls = append(decls, gen)
			}
		}
	}
	return decls, nil
}
