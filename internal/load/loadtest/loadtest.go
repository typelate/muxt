// Package loadtest builds the result of a package load without running
// the go command, for tests of what internal/load and its callers do with
// a loaded package.
//
// packages.Load runs go list, which takes seconds. What muxt reads from
// its result -- syntax, type information, the embedded files -- is plain
// data, and internal/typestest produces the type information in
// microseconds. Package writes the files to disk, because evaluating
// ParseFS and reading template sources both read them from there, and
// assembles the *packages.Package go list would have reported.
package loadtest

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/typestest"
)

// Package writes files into dir and returns them loaded as the package
// with import path pkgPath, the way load.Packages would report a load of
// dir: that package, then the standard library packages every load
// includes.
//
// Go files are type checked against the stub standard library in
// internal/typestest. Every other file is embedded, as a //go:embed
// pattern covering it would. Keys are slash-separated paths relative to
// dir; files in subdirectories are written but not part of the package.
func Package(t testing.TB, dir, pkgPath string, files map[string]string) []*packages.Package {
	t.Helper()
	goFiles := make(map[string]string)
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
			goFiles[file] = content
			goPaths = append(goPaths, file)
		default:
			embedded = append(embedded, file)
		}
	}
	slices.Sort(goPaths)
	slices.Sort(embedded)

	checked, err := typestest.CheckSyntax(pkgPath, goFiles)
	if err != nil {
		t.Fatal(err)
	}
	pl := []*packages.Package{{
		ID:         pkgPath,
		Name:       checked.Types.Name(),
		PkgPath:    pkgPath,
		Dir:        dir,
		GoFiles:    goPaths,
		EmbedFiles: embedded,
		Fset:       typestest.FileSet,
		Syntax:     checked.Syntax,
		Types:      checked.Types,
		TypesInfo:  checked.Info,
	}}
	// load.Packages always loads these alongside the working directory's
	// package, so what a route argument binds to is found even when the
	// package does not import it.
	for _, path := range []string{"encoding", "fmt", "net/http"} {
		std, ok := typestest.Lookup(path)
		if !ok {
			t.Fatalf("typestest has no stub for %s", path)
		}
		pl = append(pl, &packages.Package{ID: path, Name: std.Name(), PkgPath: path, Fset: typestest.FileSet, Types: std})
	}
	return pl
}
