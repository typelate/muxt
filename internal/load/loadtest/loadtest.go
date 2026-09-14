// Package loadtest builds the result of a package load, type checked against
// the official standard library, for tests of what internal/load and its
// callers do with a loaded package.
//
// packages.Load runs go list over the whole package graph, which takes
// seconds. What muxt reads from its result -- syntax, type information, the
// embedded files -- is plain data. Package type checks the files it is given
// in memory and imports the standard library from the go command's export
// data, which the build cache keeps: listing it costs a test binary a couple
// of hundred milliseconds once, and reading a package is shared after that. The
// standard library is whichever the go command in use provides, so the
// tests follow the toolchain rather than a copy of its API.
//
// Package writes the files to disk too, because evaluating ParseFS and
// reading template sources both read them from there.
package loadtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

// FileSet positions every package Package loads, and the standard library
// packages imported for them.
var FileSet = token.NewFileSet()

// The standard library is read from the export data the go command writes
// for it, with gcexportdata -- the reader go/packages uses -- into one map
// shared by every package this test binary loads, so a type a test names is
// the same object wherever it is reached.
//
// Export data lists only the packages a package's API mentions, while a
// load reports every package it imports; check finds fmt, whose functions a
// template calls, through html/template's imports. So each package's
// imports are set to the ones go list reports, as go/packages sets them.
var (
	importLock sync.Mutex
	stdlib     = make(map[string]*types.Package)
	imported   = make(map[string]bool)

	stdList = sync.OnceValues(func() (map[string]stdPackage, error) {
		// Tab separated: an export path holds whatever the build cache's
		// directory is called, spaces included.
		out, err := exec.Command("go", "list", "-export", "-f", "{{.ImportPath}}\t{{.Export}}\t{{join .Imports \",\"}}", "std").Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("go list -export std: %w: %s", err, exitErr.Stderr)
			}
			return nil, fmt.Errorf("go list -export std: %w", err)
		}
		list := make(map[string]stdPackage)
		for line := range strings.Lines(string(out)) {
			fields := strings.Split(strings.TrimRight(line, "\n"), "\t")
			// A package the go command builds nothing for -- the
			// standard library's own test packages -- has no export
			// data to read, so it is not one to import.
			if len(fields) < 2 || fields[1] == "" {
				continue
			}
			pkg := stdPackage{export: fields[1]}
			if len(fields) > 2 && fields[2] != "" {
				pkg.imports = strings.Split(fields[2], ",")
			}
			list[fields[0]] = pkg
		}
		return list, nil
	})
)

type stdPackage struct {
	export  string
	imports []string
}

func importStd(path string) (*types.Package, error) {
	importLock.Lock()
	defer importLock.Unlock()
	return importStdLocked(path)
}

func importStdLocked(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if pkg, ok := stdlib[path]; ok && imported[path] {
		return pkg, nil
	}
	list, err := stdList()
	if err != nil {
		return nil, err
	}
	entry, ok := list[path]
	if !ok {
		return nil, fmt.Errorf("loadtest imports only the standard library, and %q is not in it", path)
	}
	f, err := os.Open(entry.export)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r, err := gcexportdata.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("reading export data for %s: %w", path, err)
	}
	pkg, err := gcexportdata.Read(r, FileSet, stdlib, path)
	if err != nil {
		return nil, fmt.Errorf("reading export data for %s: %w", path, err)
	}
	imported[path] = true
	imports := make([]*types.Package, 0, len(entry.imports))
	for _, importPath := range entry.imports {
		if _, listed := list[importPath]; !listed && importPath != "unsafe" {
			continue
		}
		dep, err := importStdLocked(importPath)
		if err != nil {
			return nil, err
		}
		imports = append(imports, dep)
	}
	pkg.SetImports(imports)
	return pkg, nil
}

type importerFunc func(path string) (*types.Package, error)

func (fn importerFunc) Import(path string) (*types.Package, error) { return fn(path) }

// Package writes files into dir and returns them loaded as the package with
// import path pkgPath, the way load.Packages would report a load of dir: that
// package, then the standard library packages every load includes.
//
// Go files are type checked against the standard library. Every other file
// is embedded, as a //go:embed pattern covering it would. Keys are
// slash-separated paths relative to dir; files in subdirectories are written
// but not part of the package.
func Package(t testing.TB, dir, pkgPath string, files map[string]string) []*packages.Package {
	t.Helper()
	var goPaths, embedded []string
	for name, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		switch {
		case path.Dir(name) != ".":
		case strings.HasSuffix(name, ".go"):
			goPaths = append(goPaths, file)
		default:
			embedded = append(embedded, file)
		}
	}
	slices.Sort(goPaths)
	slices.Sort(embedded)

	// load.Packages always loads these alongside the working directory's
	// package, so what a route argument binds to is found even when the
	// package does not import it. They are imported before the package is
	// checked: a package the checker reaches only through another's imports
	// is read shallowly, and it is these complete ones -- fmt, whose
	// functions a template calls -- that the load's other packages share.
	roots := []string{"encoding", "fmt", "net/http"}
	std := make([]*types.Package, 0, len(roots))
	for _, stdPath := range roots {
		pkg, err := importStd(stdPath)
		if err != nil {
			t.Fatal(err)
		}
		std = append(std, pkg)
	}

	syntax := make([]*ast.File, 0, len(goPaths))
	for _, file := range goPaths {
		parsed, err := parser.ParseFile(FileSet, file, files[filepath.Base(file)], parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		syntax = append(syntax, parsed)
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
	config := types.Config{Importer: importerFunc(importStd)}
	pkg, err := config.Check(pkgPath, FileSet, syntax, info)
	if err != nil {
		t.Fatal(err)
	}

	pl := []*packages.Package{{
		ID:         pkgPath,
		Name:       pkg.Name(),
		PkgPath:    pkgPath,
		Dir:        dir,
		GoFiles:    goPaths,
		EmbedFiles: embedded,
		Fset:       FileSet,
		Syntax:     syntax,
		Types:      pkg,
		TypesInfo:  info,
	}}
	for _, pkg := range std {
		pl = append(pl, &packages.Package{ID: pkg.Path(), Name: pkg.Name(), PkgPath: pkg.Path(), Fset: FileSet, Types: pkg})
	}
	return pl
}
