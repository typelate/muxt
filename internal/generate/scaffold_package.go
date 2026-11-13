package generate

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

// PackageFileSpec is what a new package directory needs. PackageFile builds
// go/ast nodes rather than parsing source text, so the output is what the
// printer formats.
type PackageFileSpec struct {
	// Name is the package name, which is also the directory and file stem.
	Name string
	// ReceiverType is the struct type the generated routes call methods on.
	ReceiverType string
	// TemplatesVariable names the variable that holds the parsed templates,
	// the one the package's muxt.json entry reads. Empty means "templates".
	TemplatesVariable string
}

// PackageFile returns the file name and formatted source of the Go file for the
// package directory described by spec. The file declares the package, the
// embedded templates and the parsed templates, and the receiver type as an
// empty struct.
//
// It is laid out like a generated routes file: declarations carry no
// positions and formatFile decides the blank lines.
func PackageFile(spec PackageFileSpec) (filename string, src []byte, err error) {
	templatesVariable := cmp.Or(spec.TemplatesVariable, "templates")
	file := newFile(source.Package{Types: types.NewPackage("", spec.Name)})

	embedFS := astgen.ExportedIdentifier(file, "", "embed", "FS")
	parseTemplates := astgen.Call(file, "template", "html/template", "Must",
		&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   astgen.Call(file, "template", "html/template", "New", astgen.String("")),
				Sel: ast.NewIdent("ParseFS"),
			},
			Args: []ast.Expr{ast.NewIdent(templateSourceVariable), astgen.String(templateGlob)},
		})
	decls := []ast.Decl{
		&ast.GenDecl{
			Doc: &ast.CommentGroup{List: []*ast.Comment{{Text: "//go:embed " + templateGlob}}},
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ast.NewIdent(templateSourceVariable)},
				Type:  embedFS,
			}},
		},
		&ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names:  []*ast.Ident{ast.NewIdent(templatesVariable)},
				Values: []ast.Expr{parseTemplates},
			}},
		},
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name: ast.NewIdent(spec.ReceiverType),
				// The printer keeps braces on one line when both are
				// valid positions on the same line; in the empty file set
				// it prints them with, every position is line 0.
				Type: &ast.StructType{Struct: 1, Fields: &ast.FieldList{Opening: 1, Closing: 1}},
			}},
		},
	}

	// The import declaration is built last: building the other
	// declarations is what registers the imports they use.
	if err := file.Err(); err != nil {
		return "", nil, fmt.Errorf("building %s.go: %w", spec.Name, err)
	}
	filename = spec.Name + ".go"
	header, err := formatFile(filename, &ast.File{Name: ast.NewIdent(spec.Name), Decls: []ast.Decl{importDecl(file)}})
	if err != nil {
		return "", nil, err
	}
	var buf strings.Builder
	buf.WriteString(header)
	// The printer breaks lines between declarations by their positions, and
	// these have none, so each is printed as a file of its own and set apart
	// by a blank line.
	for _, decl := range decls {
		printed, err := formatFile(filename, &ast.File{Name: ast.NewIdent(spec.Name), Decls: []ast.Decl{decl}})
		if err != nil {
			return "", nil, err
		}
		buf.WriteByte('\n')
		buf.WriteString(strings.TrimPrefix(printed, "package "+spec.Name+"\n\n"))
	}
	return filename, []byte(buf.String()), nil
}

const (
	// templateSourceVariable holds the embedded template files.
	templateSourceVariable = "templateSource"
	// templateGlob is the embed pattern and the ParseFS pattern.
	templateGlob = "*.gohtml"
)
