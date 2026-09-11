package mutation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
	"text/template/parse"

	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/asteval"
)

// pageSource declares the types the typed tests render against.
const pageSource = `
type Page struct {
	Name  string
	Count int
	Price float64
	Flag  bool
	Items []Item
	Tags  map[string]Item
	Owner *User
}

type Item struct{ ID int }

type User struct{ Email string }

func (*User) Display() string { return "" }
`

// dataType type checks src as package example.com/data and returns the
// type it declares under name.
func dataType(t *testing.T, src, name string) types.Type {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "data.go", "package data\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("example.com/data", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.Scope().Lookup(name).Type()
}

func safeHTML() types.Type {
	pkg := types.NewPackage("html/template", "template")
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, "HTML", nil), types.Typ[types.String], nil)
}

// TestZeroLiteral states which actions get a typed zero value and which
// fall back to emptying, which is the difference between action-zero and
// action-empty in a report.
func TestZeroLiteral(t *testing.T) {
	page := dataType(t, pageSource, "Page")
	trusted := types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "", safeHTML())), false)
	functions := check.Functions{"trusted": trusted}

	for _, tt := range []struct {
		action string
		dot    types.Type
		want   string
		typed  bool
	}{
		{action: `{{.Name}}`, want: `""`, typed: true},
		{action: `{{.Count}}`, want: `0`, typed: true},
		{action: `{{.Price}}`, want: `0`, typed: true},
		{action: `{{.Flag}}`, want: `false`, typed: true},
		{action: `{{.Owner.Email}}`, want: `""`, typed: true},
		{action: `{{.Owner.Display}}`, want: `""`, typed: true},
		{action: `{{.}}`, dot: types.Typ[types.Int], want: `0`, typed: true},
		{action: `{{len .Items}}`, want: `0`, typed: true},
		{action: `{{.Count | printf "%d"}}`, want: `""`, typed: true},
		{action: `{{eq .Count 1}}`, want: `false`, typed: true},
		{action: `{{trusted}}`}, // a safe string has no literal
		{action: `{{.Items}}`},
		{action: `{{index .Items 0}}`},
		{action: `{{.Missing}}`},
		{action: `{{$}}`},
	} {
		t.Run(tt.action, func(t *testing.T) {
			dot := tt.dot
			if dot == nil {
				dot = page
			}
			trees, err := asteval.ParseTrees("t", tt.action, "", "", functions)
			if err != nil {
				t.Fatal(err)
			}
			pipe := trees["t"].Root.Nodes[0].(*parse.ActionNode).Pipe
			if got, typed := zeroLiteral(dot, pipe, functions); got != tt.want || typed != tt.typed {
				t.Errorf("zeroLiteral(%s) = %q, %t, want %q, %t", tt.action, got, typed, tt.want, tt.typed)
			}
		})
	}
}
