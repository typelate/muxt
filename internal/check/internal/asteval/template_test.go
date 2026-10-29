package asteval

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindPackage_nilPkg(t *testing.T) {
	pkg, ok := findPackage(nil, "fmt")
	require.False(t, ok, "findPackage should return false for nil package")
	require.Nil(t, pkg)
}

func TestEvaluateFuncMap_nonFunctionValue(t *testing.T) {
	// Create a small Go program with a non-function value in a FuncMap-like literal.
	src := `package testpkg

var notAFunc = 42
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	conf := types.Config{}
	pkg, err := conf.Check("testpkg", fset, []*ast.File{file}, nil)
	require.NoError(t, err)

	// Build a FuncMap{"f": notAFunc} call expression.
	valueIdent := &ast.Ident{Name: "notAFunc", NamePos: file.Pos()}
	kv := &ast.KeyValueExpr{
		Key:   &ast.BasicLit{Kind: token.STRING, Value: `"f"`, ValuePos: file.Pos()},
		Value: valueIdent,
	}
	lit := &ast.CompositeLit{
		Elts: []ast.Expr{kv},
	}
	call := &ast.CallExpr{
		Args:   []ast.Expr{lit},
		Lparen: file.Pos(),
	}

	fm := make(map[string]any)
	funcTypes := make(TemplateFunctions)

	err = evaluateFuncMap(".", nil, pkg, fset, call, fm, funcTypes)
	require.Error(t, err, "evaluateFuncMap should return an error for non-function FuncMap values")
	require.Contains(t, err.Error(), "not a function type")
}
