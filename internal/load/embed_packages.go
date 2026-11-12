package load

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
// An import with no module (the standard library) is skipped. An import in
// another module, or one that embeds nothing, is returned with Vars nil: it
// is described, so the evaluator can say why it cannot be read, but its files
// are not parsed. For the rest the package's Go files are parsed with
// comments into pkg's token.FileSet, because the //go:embed comment that
// decides which files a variable holds is on the declaration, and the load
// does not keep the syntax of an import.
func embedPackages(pkg *packages.Package) (map[string]source.EmbedPackage, error) {
	result := make(map[string]source.EmbedPackage)
	for path, imported := range pkg.Imports {
		if imported.Module == nil || len(imported.GoFiles) == 0 {
			continue
		}
		ep := source.EmbedPackage{
			ImportPath: imported.PkgPath,
			ModulePath: imported.Module.Path,
			Dir:        filepath.Dir(imported.GoFiles[0]),
			EmbedFiles: slices.Sorted(slices.Values(imported.EmbedFiles)),
		}
		if pkg.Module != nil && imported.Module.Path == pkg.Module.Path && len(imported.EmbedFiles) > 0 {
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
