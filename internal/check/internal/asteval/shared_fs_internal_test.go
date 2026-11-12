package asteval

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/source"
)

// sharedArgument parses a workspace file that calls ParseFS on the selector
// fragments.name, and returns a scope that imports the fragments package of
// module example.com/m, with the selector the ParseFS call's first argument.
func sharedArgument(t *testing.T, name string) (Scope, *ast.SelectorExpr) {
	t.Helper()
	dir := filepath.FromSlash("/m/workspace")
	fileSet := token.NewFileSet()
	src := "package workspace\n\nimport \"example.com/m/fragments\"\n\nvar _ = fragments." + name + "\n"
	file, err := parser.ParseFile(fileSet, filepath.Join(dir, "workspace.go"), src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var selector *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && selector == nil {
			selector = sel
		}
		return true
	})
	info := &types.Info{Uses: make(map[*ast.Ident]types.Object)}
	pkg := types.NewPackage("example.com/m/fragments", "fragments")
	info.Uses[selector.X.(*ast.Ident)] = types.NewPkgName(token.NoPos, nil, "fragments", pkg)
	scope := Scope{
		Fset:       fileSet,
		Info:       info,
		Dir:        dir,
		ModulePath: "example.com/m",
		Imports: map[string]source.EmbedPackage{
			"example.com/m/fragments": {ImportPath: "example.com/m/fragments", ModulePath: "example.com/m", Dir: filepath.FromSlash("/m/fragments")},
		},
	}
	return scope, selector
}

// TestSharedFilesUnexportedVariable states that a ParseFS argument naming an
// unexported variable of another package fails with an UnexportedSharedFSError
// that names the selector and the position of the argument.
func TestSharedFilesUnexportedVariable(t *testing.T) {
	scope, selector := sharedArgument(t, "templateSource")

	_, _, err := sharedFiles(scope, selector)

	var typed UnexportedSharedFSError
	if !errors.As(err, &typed) {
		t.Fatalf("sharedFiles(fragments.templateSource) error = %v (%T), want UnexportedSharedFSError", err, err)
	}
	const want = "workspace.go:5:9: fragments.templateSource is unexported; ParseFS reads only an exported embed.FS variable of another package"
	if got := err.Error(); got != want {
		t.Errorf("sharedFiles(fragments.templateSource) error = %q, want %q", got, want)
	}
	if typed.Package != "fragments" || typed.Name != "templateSource" {
		t.Errorf("UnexportedSharedFSError{Package: %q, Name: %q}, want fragments and templateSource", typed.Package, typed.Name)
	}
	if !strings.HasSuffix(typed.Position.Filename, "workspace.go") {
		t.Errorf("UnexportedSharedFSError.Position.Filename = %q, want workspace.go", typed.Position.Filename)
	}
}
