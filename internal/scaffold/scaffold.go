// Package scaffold builds the Go source of a new muxt package directory. It
// builds go/ast nodes rather than parsing source text, so the output is what
// the printer formats.
package scaffold

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
)

// PackageSpec is what a new package directory needs.
type PackageSpec struct {
	// Name is the package name, which is also the directory and file stem.
	Name string
	// ReceiverType is the struct type the generated routes call methods on.
	ReceiverType string
}

// Package returns the file name and formatted source of the Go file for the
// package directory described by spec. The file declares the package and the
// receiver type as an empty struct.
func Package(spec PackageSpec) (filename string, src []byte, err error) {
	file := &ast.File{
		Name: ast.NewIdent(spec.Name),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{&ast.TypeSpec{
					Name: ast.NewIdent(spec.ReceiverType),
					Type: &ast.StructType{Fields: &ast.FieldList{}},
				}},
			},
		},
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), file); err != nil {
		return "", nil, fmt.Errorf("formatting %s.go: %w", spec.Name, err)
	}
	return spec.Name + ".go", buf.Bytes(), nil
}
