package generate

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// formatFile prints a generated file and formats it with go/format.
//
// The imports are the file's own: each file registers the packages its
// declarations reference as it builds them, so there is nothing to add or
// remove. They are laid out the way gofmt users expect, the standard
// library first and every other path after it, each group sorted and set
// apart by a blank line.
func formatFile(filePath string, f *ast.File) (string, error) {
	var imports []*ast.ImportSpec
	decls := f.Decls[:0:0]
	for _, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			for _, spec := range gen.Specs {
				imports = append(imports, spec.(*ast.ImportSpec))
			}
			continue
		}
		decls = append(decls, decl)
	}
	body := *f
	body.Decls = decls

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "package %s\n\n", f.Name.Name)
	if err := writeImports(&buf, imports); err != nil {
		return "", fmt.Errorf("formatting %s: %w", filePath, err)
	}
	var rest bytes.Buffer
	if err := printer.Fprint(&rest, token.NewFileSet(), &body); err != nil {
		return "", fmt.Errorf("formatting %s: %w", filePath, err)
	}
	// The printed file repeats the package clause written above.
	_, afterClause, _ := bytes.Cut(rest.Bytes(), []byte("\n"))
	buf.Write(afterClause)

	out, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("formatting %s: %w", filePath, err)
	}
	return string(bytes.ReplaceAll(out, []byte("\n}\nfunc "), []byte("\n}\n\nfunc "))), nil
}

// writeImports writes an import declaration for specs, grouped by
// importGroup and sorted by path within each group, with a blank line
// between groups.
func writeImports(buf *bytes.Buffer, specs []*ast.ImportSpec) error {
	type entry struct {
		name, path string
		group      int
	}
	entries := make([]entry, 0, len(specs))
	for _, spec := range specs {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return err
		}
		e := entry{path: path, group: importGroup(path)}
		if spec.Name != nil {
			e.name = spec.Name.Name
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.path, b.path), cmp.Compare(a.name, b.name))
	})
	entries = slices.Compact(entries)

	line := func(e entry) string {
		if e.name != "" {
			return e.name + " " + strconv.Quote(e.path)
		}
		return strconv.Quote(e.path)
	}
	switch len(entries) {
	case 0:
		return nil
	case 1:
		fmt.Fprintf(buf, "import %s\n\n", line(entries[0]))
		return nil
	}
	buf.WriteString("import (\n")
	for i, e := range entries {
		if i > 0 && e.group != entries[i-1].group {
			buf.WriteString("\n")
		}
		buf.WriteString("\t" + line(e) + "\n")
	}
	buf.WriteString(")\n\n")
	return nil
}

// importGroup orders an import path: the standard library, whose paths have
// no dot in their first element, before everything else.
func importGroup(path string) int {
	first, _, _ := strings.Cut(path, "/")
	if strings.Contains(first, ".") {
		return 1
	}
	return 0
}
