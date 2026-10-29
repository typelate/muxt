package check

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRenderTypeSource_IncludesGodoc loads a real on-disk Go source file
// and confirms that the rendered declaration contains the type's leading
// godoc comment.
func TestRenderTypeSource_IncludesGodoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.go")
	src := `package app

// Page is the home page model.
//
// More text on the second line.
type Page struct {
	Title string
}
`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	require.NoError(t, err)

	conf := types.Config{}
	pkg, err := conf.Check("app", fset, []*ast.File{parsed}, nil)
	require.NoError(t, err)

	pageType := pkg.Scope().Lookup("Page").Type()

	got := renderTypeSource(pageType, fset)
	require.Contains(t, got, "// Page is the home page model.")
	require.Contains(t, got, "// More text on the second line.")
	require.Contains(t, got, "type Page struct {")
	require.Contains(t, got, "Title string")
	require.NotContains(t, got, "(\n", "single-spec declarations should not be wrapped in parens")
}

func TestRenderTypeSource_NoGodoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.go")
	src := `package app

type Page struct {
	Title string
}
`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	require.NoError(t, err)

	conf := types.Config{}
	pkg, err := conf.Check("app", fset, []*ast.File{parsed}, nil)
	require.NoError(t, err)

	pageType := pkg.Scope().Lookup("Page").Type()

	got := renderTypeSource(pageType, fset)
	require.Contains(t, got, "type Page struct {")
	require.Contains(t, got, "Title string")
}

func TestRenderTypeSource_NotNamed(t *testing.T) {
	require.Equal(t, "", renderTypeSource(types.Universe.Lookup("int").Type(), token.NewFileSet()))
}

func TestRenderTypeSource_NilFset(t *testing.T) {
	require.Equal(t, "", renderTypeSource(nil, nil))
}
