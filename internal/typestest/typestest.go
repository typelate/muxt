// Package typestest type checks Go source in memory, for tests that need
// go/types values without a module on disk.
//
// Loading a real package runs the go command, and type checking the
// standard library from source takes seconds: net/http alone pulls in
// most of it. Route resolution and generation only ever ask a handful of
// questions of the standard library -- is this parameter an
// *http.Request, does this type implement encoding.TextUnmarshaler -- so
// the packages here are stubs declaring just the API muxt reads, with
// the real import paths and names. Checking them takes microseconds.
//
// A stub declares an identifier with the signature the real package
// gives it, or not at all. Add to one when a test needs more of it.
package typestest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"sync"
	"testing"
)

// FileSet positions the stub packages and every package Check type
// checks. It is safe for concurrent use.
var FileSet = token.NewFileSet()

// Check type checks files as the package with the import path path. Each
// entry maps a file name to its source. Imports resolve to the stub
// standard library packages; importing anything else is an error.
func Check(path string, files map[string]string) (*types.Package, error) {
	checked, err := CheckSyntax(path, files)
	if err != nil {
		return nil, err
	}
	return checked.Types, nil
}

// Checked is a package type checked by CheckSyntax, with the syntax and
// type information a test walks to find what the source does.
type Checked struct {
	Types  *types.Package
	Syntax []*ast.File
	Info   *types.Info
}

// CheckSyntax is Check, also returning the parsed files, in file name
// order, and the type information recorded for them.
func CheckSyntax(path string, files map[string]string) (*Checked, error) {
	std, err := stdlib()
	if err != nil {
		return nil, err
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Instances:  make(map[*ast.Ident]types.Instance),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}
	pkg, syntax, err := checkWithInfo(path, files, importerFunc(func(p string) (*types.Package, error) {
		if pkg, ok := std[p]; ok {
			return pkg, nil
		}
		return nil, fmt.Errorf("typestest has no stub for package %q", p)
	}), info)
	if err != nil {
		return nil, err
	}
	return &Checked{Types: pkg, Syntax: syntax, Info: info}, nil
}

// MustCheck is Check for a single file, failing t when the source does
// not type check.
func MustCheck(t testing.TB, path, src string) *types.Package {
	t.Helper()
	pkg, err := Check(path, map[string]string{"source.go": src})
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

// Lookup finds a stub standard library package by import path.
func Lookup(path string) (*types.Package, bool) {
	std, err := stdlib()
	if err != nil {
		panic(err)
	}
	pkg, ok := std[path]
	return pkg, ok
}

// Paths lists the import paths of the stub packages.
func Paths() []string {
	paths := make([]string, 0, len(stubs))
	for path := range stubs {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Type returns the type the stub package at path declares as name,
// panicking when there is none: a test naming a type is asserting the
// stub declares it.
func Type(path, name string) types.Type {
	pkg, ok := Lookup(path)
	if !ok {
		panic(fmt.Sprintf("typestest has no stub for package %q", path))
	}
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		panic(fmt.Sprintf("typestest stub %q does not declare %s", path, name))
	}
	return obj.Type()
}

type importerFunc func(path string) (*types.Package, error)

func (fn importerFunc) Import(path string) (*types.Package, error) { return fn(path) }

func check(path string, files map[string]string, importer types.Importer) (*types.Package, error) {
	pkg, _, err := checkWithInfo(path, files, importer, nil)
	return pkg, err
}

func checkWithInfo(path string, files map[string]string, importer types.Importer, info *types.Info) (*types.Package, []*ast.File, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	syntax := make([]*ast.File, 0, len(files))
	for _, name := range names {
		file, err := parser.ParseFile(FileSet, name, files[name], parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, err
		}
		syntax = append(syntax, file)
	}
	config := types.Config{Importer: importer}
	pkg, err := config.Check(path, FileSet, syntax, info)
	return pkg, syntax, err
}

var stdlib = sync.OnceValues(func() (map[string]*types.Package, error) {
	checked := make(map[string]*types.Package, len(stubs))
	var importing []string
	var importPath func(path string) (*types.Package, error)
	importPath = func(path string) (*types.Package, error) {
		if pkg, ok := checked[path]; ok {
			return pkg, nil
		}
		src, ok := stubs[path]
		if !ok {
			return nil, fmt.Errorf("typestest stub imports %q, which has no stub", path)
		}
		if slices.Contains(importing, path) {
			return nil, fmt.Errorf("typestest stubs import each other in a cycle: %v", append(importing, path))
		}
		importing = append(importing, path)
		defer func() { importing = importing[:len(importing)-1] }()
		pkg, err := check(path, map[string]string{path + ".go": src}, importerFunc(importPath))
		if err != nil {
			return nil, fmt.Errorf("typestest stub %q: %w", path, err)
		}
		checked[path] = pkg
		return pkg, nil
	}
	for _, path := range Paths() {
		if _, err := importPath(path); err != nil {
			return nil, err
		}
	}
	return checked, nil
})

// Packages returns the stub standard library packages by import path, the
// shape of source.Package's Imports. The map is a copy; the packages are
// shared.
func Packages() map[string]*types.Package {
	std, err := stdlib()
	if err != nil {
		panic(err)
	}
	packages := make(map[string]*types.Package, len(std))
	for path, pkg := range std {
		packages[path] = pkg
	}
	return packages
}
