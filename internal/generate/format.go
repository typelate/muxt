package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"strconv"

	"golang.org/x/tools/imports"
)

// formatFile prints a generated file, dropping the imports it does not
// use, and formats it the way goimports does.
//
// Generation registers every import it references, but the registry is
// shared by all the files of one run and some registrations are for
// expressions later discarded, so a file can carry imports it does not
// use. goimports would drop them, but it also looks for a package to
// import for every selector it cannot resolve -- and the templates
// variable, declared in another file, is one -- which means walking the
// module cache and the file's directory. Generation never needs an import
// found, so the unused ones are dropped here and goimports only formats.
func formatFile(filePath string, f *ast.File) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), f); err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	used, err := packageReferences(filePath, buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	if removeUnusedImports(f, used) {
		buf.Reset()
		if err := printer.Fprint(&buf, token.NewFileSet(), f); err != nil {
			return "", fmt.Errorf("formatting error: %v", err)
		}
	}
	out, err := imports.Process(filePath, buf.Bytes(), &imports.Options{
		Fragment:   true,
		AllErrors:  true,
		Comments:   true,
		FormatOnly: true,
	})
	if err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	return string(bytes.ReplaceAll(out, []byte("\n}\nfunc "), []byte("\n}\n\nfunc "))), nil
}

// packageReferences names the identifiers src selects from that do not
// resolve to a declaration in the file: the package names it refers to,
// or a package-level identifier declared in another file, which no
// import is named after.
func packageReferences(filePath string, src []byte) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, src, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil {
			used[id.Name] = true
		}
		return true
	})
	return used, nil
}

// removeUnusedImports deletes the import specs whose name is not in used
// and reports whether it deleted any. An import without an explicit name
// is named after the last element of its path: generation names an import
// explicitly whenever the two differ.
func removeUnusedImports(f *ast.File, used map[string]bool) bool {
	removed := false
	decls := f.Decls[:0]
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			decls = append(decls, decl)
			continue
		}
		specs := gen.Specs[:0]
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			if used[importName(imp)] {
				specs = append(specs, spec)
				continue
			}
			removed = true
		}
		gen.Specs = specs
		if len(specs) > 0 {
			decls = append(decls, gen)
		}
	}
	f.Decls = decls
	return removed
}

func importName(imp *ast.ImportSpec) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	p, err := strconv.Unquote(imp.Path.Value)
	if err != nil {
		return ""
	}
	return path.Base(p)
}
