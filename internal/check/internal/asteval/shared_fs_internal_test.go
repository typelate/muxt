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

// sharedArgument parses a workspace file of module example.com/m whose ParseFS
// first argument is the selector fragments.name, and returns a scope that
// imports importPath, a package of modulePath, with the selector typed as typ,
// the way the analyzed package's type information records it.
func sharedArgument(t *testing.T, name string, typ types.Type, importPath, modulePath string) (Scope, *ast.SelectorExpr) {
	t.Helper()
	dir := filepath.FromSlash("/m/workspace")
	fileSet := token.NewFileSet()
	src := "package workspace\n\nimport \"" + importPath + "\"\n\nvar _ = fragments." + name + "\n"
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
	pkg := types.NewPackage(importPath, "fragments")
	info.Uses[selector.X.(*ast.Ident)] = types.NewPkgName(token.NoPos, nil, "fragments", pkg)
	info.Uses[selector.Sel] = types.NewVar(token.NoPos, pkg, name, typ)
	scope := Scope{
		Fset:       fileSet,
		Info:       info,
		Dir:        dir,
		ModulePath: "example.com/m",
		Imports: map[string]source.EmbedPackage{
			importPath: {ImportPath: importPath, ModulePath: modulePath, Dir: filepath.FromSlash("/m/fragments")},
		},
	}
	return scope, selector
}

// TestSharedFilesUnexportedVariable states that a ParseFS argument naming an
// unexported variable of another package fails with an UnexportedSharedFSError
// that names the selector and the position of the argument.
func TestSharedFilesUnexportedVariable(t *testing.T) {
	scope, selector := sharedArgument(t, "templateSource", namedType("embed", "FS"), "example.com/m/fragments", "example.com/m")

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
	scope, selector := sharedArgument(t, "Other", namedType("io/fs", "FS"), "example.com/m/fragments", "example.com/m")

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

// TestSharedFilesOtherModuleError states that a ParseFS argument naming a
// package outside the analyzed package's module fails with an OtherModuleError
// that names both the import path and this module.
func TestSharedFilesOtherModuleError(t *testing.T) {
	scope, selector := sharedArgument(t, "TemplateSource", namedType("embed", "FS"), "example.org/other/fragments", "example.org/other")

	_, _, err := sharedFiles(scope, selector)

	var typed OtherModuleError
	if !errors.As(err, &typed) {
		t.Fatalf("sharedFiles(fragments.TemplateSource) error = %v (%T), want OtherModuleError", err, err)
	}
	const want = "workspace.go:5:9: package example.org/other/fragments is in module example.org/other, not in this module (example.com/m); a shared embed.FS must come from a package of this module"
	if got := err.Error(); got != want {
		t.Errorf("sharedFiles(fragments.TemplateSource) error = %q, want %q", got, want)
	}
	if typed.ImportPath != "example.org/other/fragments" || typed.ThisModule != "example.com/m" {
		t.Errorf("OtherModuleError{ImportPath: %q, ThisModule: %q}, want example.org/other/fragments and example.com/m", typed.ImportPath, typed.ThisModule)
	}
}

// TestSharedFilesStandardLibraryPackage states that a ParseFS argument naming a
// standard library package, which has no module path, says the package holds
// no embed.FS muxt can read instead of naming an empty module.
func TestSharedFilesStandardLibraryPackage(t *testing.T) {
	scope, selector := sharedArgument(t, "TemplateSource", namedType("embed", "FS"), "net/http", "")

	_, _, err := sharedFiles(scope, selector)

	var typed OtherModuleError
	if !errors.As(err, &typed) {
		t.Fatalf("sharedFiles(fragments.TemplateSource) error = %v (%T), want OtherModuleError", err, err)
	}
	const want = "workspace.go:5:9: package net/http is in the standard library, which holds no embed.FS muxt can read; a shared embed.FS must come from a package of this module (example.com/m)"
	if got := err.Error(); got != want {
		t.Errorf("sharedFiles(fragments.TemplateSource) error = %q, want %q", got, want)
	}
}

// TestSharedFilesUnavailablePackage states that a ParseFS argument naming a
// package the evaluation was given no embedded files for fails with a
// SharedPackageUnavailableError, not an OtherModuleError, and that the message
// never prints empty parentheses.
func TestSharedFilesUnavailablePackage(t *testing.T) {
	scope, selector := sharedArgument(t, "TemplateSource", namedType("embed", "FS"), "example.com/m/fragments", "example.com/m")
	scope.Imports = nil
	scope.ModulePath = ""

	_, _, err := sharedFiles(scope, selector)

	var typed SharedPackageUnavailableError
	if !errors.As(err, &typed) {
		t.Fatalf("sharedFiles(fragments.TemplateSource) error = %v (%T), want SharedPackageUnavailableError", err, err)
	}
	const want = "workspace.go:5:9: muxt has no embedded files for package example.com/m/fragments; run muxt on the module so its packages are loaded together"
	if got := err.Error(); got != want {
		t.Errorf("sharedFiles(fragments.TemplateSource) error = %q, want %q", got, want)
	}
	if typed.ImportPath != "example.com/m/fragments" {
		t.Errorf("SharedPackageUnavailableError.ImportPath = %q, want example.com/m/fragments", typed.ImportPath)
	}
}
