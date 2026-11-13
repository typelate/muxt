package load

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeModule writes files, keyed by slash-separated path, under a new
// temporary directory and returns it with symbolic links resolved, so paths
// the go command reports compare equal to paths built from it.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GOWORK", "off")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err, "EvalSymlinks")
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

// TestEmbedPackagesDescribesImportedPackage states that the map holds an
// imported package of the module with its module path, directory, sorted
// absolute embedded files and the declaration of its exported embed.FS, read
// from the load that produced the package, not from a second one.
func TestEmbedPackagesDescribesImportedPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.24\n",
		"fragments/fragments.go": `package fragments

import "embed"

//go:embed *.gohtml
var TemplateSource embed.FS
`,
		"fragments/layout.gohtml": `{{define "layout"}}{{end}}`,
		"fragments/a.gohtml":      `{{define "a"}}{{end}}`,
		"fragments/notes.txt":     `{{define "draft"}}{{end}}`,
		"workspace/workspace.go": `package workspace

import _ "example.com/m/fragments"
`,
	})
	_, pl, err := Packages(filepath.Join(dir, "workspace"))
	require.NoError(t, err, "Packages")
	pkg, ok := packageInDirectory(pl, filepath.Join(dir, "workspace"))
	require.True(t, ok, "packageInDirectory")

	got, err := embedPackages(pkg)

	require.NoError(t, err, "embedPackages")
	fragments, ok := got["example.com/m/fragments"]
	require.True(t, ok, "embedPackages() has no example.com/m/fragments: %v", got)
	assert.Equal(t, "example.com/m", fragments.ModulePath, "ModulePath")
	assert.Equal(t, filepath.Join(dir, "fragments"), fragments.Dir, "Dir")
	assert.Equal(t, []string{
		filepath.Join(dir, "fragments", "a.gohtml"),
		filepath.Join(dir, "fragments", "layout.gohtml"),
	}, fragments.EmbedFiles, "EmbedFiles")
	require.Len(t, fragments.Vars, 1, "Vars")
	spec, ok := fragments.Vars[0].Specs[0].(*ast.ValueSpec)
	require.True(t, ok, "Vars[0].Specs[0] is %T", fragments.Vars[0].Specs[0])
	assert.Equal(t, "TemplateSource", spec.Names[0].Name, "declared variable")
	assert.Equal(t, filepath.Join(dir, "fragments", "fragments.go"), pkg.Fset.Position(spec.Pos()).Filename,
		"the declaration is positioned in the load's token.FileSet")
}

// TestEmbedPackagesReportsFirstImportByPath states that when several imports
// that embed files fail to parse, the error names the one first by import
// path, on every run, not whichever the imports map yields first.
func TestEmbedPackagesReportsFirstImportByPath(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	broken := func(name string) string {
		return "package " + name + "\n\nimport \"embed\"\n\n//go:embed *.gohtml\nvar FS embed.FS\n\nvar X = (\n"
	}
	dir := writeModule(t, map[string]string{
		"go.mod":                 "module example.com/m\n\ngo 1.24\n",
		"a/a.go":                 broken("a"),
		"a/a.gohtml":             "",
		"b/b.go":                 broken("b"),
		"b/b.gohtml":             "",
		"c/c.go":                 broken("c"),
		"c/c.gohtml":             "",
		"workspace/workspace.go": "package workspace\n\nimport (\n\t_ \"example.com/m/a\"\n\t_ \"example.com/m/b\"\n\t_ \"example.com/m/c\"\n)\n",
	})
	_, pl, err := Packages(filepath.Join(dir, "workspace"))
	require.NoError(t, err, "Packages")
	pkg, ok := packageInDirectory(pl, filepath.Join(dir, "workspace"))
	require.True(t, ok, "packageInDirectory")

	for range 20 {
		_, err := embedPackages(pkg)

		require.Error(t, err, "embedPackages")
		assert.ErrorContains(t, err, "reading package example.com/m/a:", "embedPackages error")
	}
}

// sharedFSModule is a module whose workspace package parses the files of
// argument, written as the first argument of ParseFS, and imports lib, a
// package of the same module that declares extra.
func sharedFSModule(t *testing.T, argument, extra string) (dir string) {
	t.Helper()
	return writeModule(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.24\n",
		"lib/lib.go": `package lib

import "embed"

` + extra + `
`,
		"workspace/workspace.go": `package workspace

import (
	"html/template"
	"os"

	"example.com/m/lib"
)

var (
	_ = os.Stdin
	_ = lib.FS
)

var templates = template.Must(template.ParseFS(` + argument + `, "*.gohtml"))
`,
		"workspace/workspace.gohtml": `{{define "GET / Home()"}}{{end}}`,
	})
}

// TestPackageSharedFSInStandardLibrary states that a ParseFS argument naming a
// variable of a standard library package gets the standard-library message,
// which says a shared embed.FS must come from this module, and not the advice
// to run muxt on the module, which a run on the module has already followed.
func TestPackageSharedFSInStandardLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := sharedFSModule(t, "os.Stdin", "var FS embed.FS")
	_, pl, err := Packages(filepath.Join(dir, "workspace"))
	require.NoError(t, err, "Packages")

	_, err = Package(filepath.Join(dir, "workspace"), pl, []string{"templates"})

	require.Error(t, err, "Package")
	assert.ErrorContains(t, err, "package os is in the standard library", "Package error")
	assert.NotContains(t, err.Error(), "run muxt on the module", "Package error")
}

// TestPackageSharedFSWithoutEmbedDirective states that a ParseFS argument
// naming an exported embed.FS of a package of this module that has no
// //go:embed directive says so, instead of saying the variable is not found.
func TestPackageSharedFSWithoutEmbedDirective(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := sharedFSModule(t, "lib.FS", "var FS embed.FS")
	_, pl, err := Packages(filepath.Join(dir, "workspace"))
	require.NoError(t, err, "Packages")

	_, err = Package(filepath.Join(dir, "workspace"), pl, []string{"templates"})

	require.Error(t, err, "Package")
	assert.ErrorContains(t, err, "lib.FS has no //go:embed directive, so it holds no files", "Package error")
	assert.NotContains(t, err.Error(), "not found", "Package error")
}

// TestPackageImportThatDoesNotParse states that a package of this module that
// embeds nothing is not parsed, so a syntax error in an import no ParseFS
// call names does not fail the load.
func TestPackageImportThatDoesNotParse(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := writeModule(t, map[string]string{
		"go.mod":                     "module example.com/m\n\ngo 1.24\n",
		"a/a.go":                     "package a\n\nvar A = (\n",
		"workspace/workspace.go":     "package workspace\n\nimport (\n\t\"embed\"\n\t\"html/template\"\n\n\t_ \"example.com/m/a\"\n)\n\n//go:embed *.gohtml\nvar source embed.FS\n\nvar templates = template.Must(template.ParseFS(source, \"*.gohtml\"))\n",
		"workspace/workspace.gohtml": `{{define "GET / Home()"}}{{end}}`,
	})
	_, pl, err := Packages(filepath.Join(dir, "workspace"))
	require.NoError(t, err, "Packages")

	_, err = Package(filepath.Join(dir, "workspace"), pl, []string{"templates"})

	require.NoError(t, err, "Package")
}
