package asteval

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringExpression(t *testing.T) {
	const src = `package p

const pattern = "*.gohtml"
const alias = pattern
var name = "site"
var cycleA = cycleB
var cycleB = cycleA
var computed = build()
const number = 42

func build() string { return "" }
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "p.go", src, 0)
	require.NoError(t, err)
	files := []*ast.File{file}

	for _, tt := range []struct {
		name string
		expr string
		want string
		err  string
	}{
		{name: "literal", expr: `"x"`, want: "x"},
		{name: "const", expr: `pattern`, want: "*.gohtml"},
		{name: "const alias", expr: `alias`, want: "*.gohtml"},
		{name: "var", expr: `name`, want: "site"},
		{name: "unknown", expr: `missing`, err: "expected string literal got missing"},
		{name: "cycle", expr: `cycleA`, err: "expected string literal got cycleA"},
		{name: "computed", expr: `computed`, err: "expected string literal got computed"},
		{name: "non string", expr: `number`, err: "expected string literal got number"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			exp, err := parser.ParseExpr(tt.expr)
			require.NoError(t, err)
			got, err := stringExpression(".", set, files, exp)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
