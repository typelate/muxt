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

// namedType is the named type path.name, as the analyzed package's type
// information names a variable's type.
func namedType(path, name string) types.Type {
	pkg := types.NewPackage(path, filepath.Base(path))
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), types.NewInterfaceType(nil, nil).Complete(), nil)
}

// sharedArgument parses a workspace file whose ParseFS first argument is the
// selector fragments.name, and returns a scope that imports the fragments
// package of module example.com/m, with the selector typed as typ, the way the
// analyzed package's type information records it.
func sharedArgument(t *testing.T, name string, typ types.Type) (Scope, *ast.SelectorExpr) {
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
	info.Uses[selector.Sel] = types.NewVar(token.NoPos, pkg, name, typ)
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
	scope, selector := sharedArgument(t, "templateSource", namedType("embed", "FS"))

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

// TestSharedFilesNotEmbedFSVariable states that a ParseFS argument naming an
// exported variable of another package whose type is not embed.FS fails with a
// NotEmbedFSError that prints the type as go/types names it.
func TestSharedFilesNotEmbedFSVariable(t *testing.T) {
	scope, selector := sharedArgument(t, "Other", namedType("io/fs", "FS"))

	_, _, err := sharedFiles(scope, selector)

	var typed NotEmbedFSError
	if !errors.As(err, &typed) {
		t.Fatalf("sharedFiles(fragments.Other) error = %v (%T), want NotEmbedFSError", err, err)
	}
	const want = "workspace.go:5:9: fragments.Other is a io/fs.FS, not an embed.FS; ParseFS reads an exported embed.FS variable of another package"
	if got := err.Error(); got != want {
		t.Errorf("sharedFiles(fragments.Other) error = %q, want %q", got, want)
	}
	if typed.Type != "io/fs.FS" {
		t.Errorf("NotEmbedFSError.Type = %q, want io/fs.FS", typed.Type)
	}
}
