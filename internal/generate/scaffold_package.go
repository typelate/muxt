package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strconv"
)

// PackageFileSpec is what a new package directory needs. PackageFile builds
// go/ast nodes rather than parsing source text, so the output is what the
// printer formats.
type PackageFileSpec struct {
	// Name is the package name, which is also the directory and file stem.
	Name string
	// ReceiverType is the struct type the generated routes call methods on.
	ReceiverType string
}

// formattedLines is how many lines the file has once printed: the package
// clause, the import block, the directive and its variable, the parsed
// templates, and the receiver type, with a blank line between each group.
const formattedLines = 13

// PackageFile returns the file name and formatted source of the Go file for the
// package directory described by spec. The file declares the package, the
// embedded templates and the parsed templates, and the receiver type as an
// empty struct.
//
// The printer decides blank lines and where a comment goes from positions, so
// each node is placed on the line it has in the formatted file.
func PackageFile(spec PackageFileSpec) (filename string, src []byte, err error) {
	fset := token.NewFileSet()
	tf := fset.AddFile(spec.Name+".go", -1, (formattedLines+1)*10)
	offsets := make([]int, formattedLines+1)
	for i := range offsets {
		offsets[i] = i * 10
	}
	if !tf.SetLines(offsets) {
		return "", nil, fmt.Errorf("laying out %s.go", spec.Name)
	}
	b := builder{file: tf}

	embedDoc := &ast.CommentGroup{List: []*ast.Comment{{Slash: b.pos(8), Text: "//go:embed *.gohtml"}}}
	file := &ast.File{
		Package: b.pos(1),
		Name:    b.ident(spec.Name, 1),
		Decls: []ast.Decl{
			&ast.GenDecl{
				TokPos: b.pos(3),
				Tok:    token.IMPORT,
				Lparen: b.pos(3),
				Specs: []ast.Spec{
					&ast.ImportSpec{Path: b.str("embed", 4)},
					&ast.ImportSpec{Path: b.str("html/template", 5)},
				},
				Rparen: b.pos(6),
			},
			&ast.GenDecl{
				TokPos: b.pos(9),
				Tok:    token.VAR,
				Doc:    embedDoc,
				Specs:  []ast.Spec{b.valueSpec("templateSource", b.selector("embed", "FS", 9), nil, 9)},
			},
			&ast.GenDecl{
				TokPos: b.pos(11),
				Tok:    token.VAR,
				Specs:  []ast.Spec{b.valueSpec("templates", nil, b.parseTemplates(11), 11)},
			},
			&ast.GenDecl{
				TokPos: b.pos(13),
				Tok:    token.TYPE,
				Specs: []ast.Spec{&ast.TypeSpec{
					Name: b.ident(spec.ReceiverType, 13),
					Type: &ast.StructType{
						Struct: b.pos(13),
						Fields: &ast.FieldList{Opening: b.pos(13), Closing: b.pos(13)},
					},
				}},
			},
		},
		Comments: []*ast.CommentGroup{embedDoc},
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return "", nil, fmt.Errorf("formatting %s.go: %w", spec.Name, err)
	}
	return spec.Name + ".go", buf.Bytes(), nil
}

// builder makes nodes that sit on a line of the file it lays out.
type builder struct {
	file *token.File
}

// pos is the position of the start of line, counted from 1.
func (b builder) pos(line int) token.Pos {
	return b.file.LineStart(line)
}

// ident is the identifier name on line.
func (b builder) ident(name string, line int) *ast.Ident {
	return &ast.Ident{NamePos: b.pos(line), Name: name}
}

// str is the Go string literal for s on line.
func (b builder) str(s string, line int) *ast.BasicLit {
	return &ast.BasicLit{ValuePos: b.pos(line), Kind: token.STRING, Value: strconv.Quote(s)}
}

// selector is the qualified identifier pkg.name on line.
func (b builder) selector(pkg, name string, line int) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: b.ident(pkg, line), Sel: b.ident(name, line)}
}

// parseTemplates is template.Must(template.New("").ParseFS(templateSource, "*.gohtml")) on line.
func (b builder) parseTemplates(line int) ast.Expr {
	parse := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun:  b.selector("template", "New", line),
				Args: []ast.Expr{b.str("", line)},
			},
			Sel: b.ident("ParseFS", line),
		},
		Args: []ast.Expr{b.ident("templateSource", line), b.str("*.gohtml", line)},
	}
	return &ast.CallExpr{Fun: b.selector("template", "Must", line), Args: []ast.Expr{parse}}
}

// valueSpec declares name, with an optional type and an optional value, on line.
func (b builder) valueSpec(name string, typ, value ast.Expr, line int) *ast.ValueSpec {
	spec := &ast.ValueSpec{Names: []*ast.Ident{b.ident(name, line)}, Type: typ}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return spec
}
