package cli

import (
	"bytes"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/composite"
)

// entryModule writes a module whose packages are the keys of receivers, each
// mapped to the receiver type its routes call a method of, and a muxt.json
// with an entry for every one. A package imports the packages named in
// imports, keyed the same way. It returns the module's directory.
func entryModule(t *testing.T, receivers map[string]string, imports map[string][]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	entries := make(map[string]string)
	for key, receiver := range receivers {
		pkgDir := filepath.Join(dir, filepath.FromSlash(key))
		require.NoError(t, os.MkdirAll(pkgDir, 0o755))
		var blank strings.Builder
		for _, imp := range imports[key] {
			fmt.Fprintf(&blank, "\t_ %q\n", "example.com/m/"+imp)
		}
		name := filepath.Base(pkgDir)
		if key == "." {
			name = "root"
		}
		writeTestFile(t, pkgDir, "package.go", fmt.Sprintf(`package %s

import (
	"embed"
	"html/template"

%s)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type %s struct{}

func (%s) %sPage() any { return nil }
`, name, blank.String(), receiver, receiver, receiver))
		writeTestFile(t, pkgDir, "page.gohtml", fmt.Sprintf(`{{define "GET /%s %sPage()"}}<p>{{.}}</p>{{end}}`, receiver, receiver))
		entries[key] = fmt.Sprintf(`%q: {"use_receiver_type": %q}`, importPath(key), receiver)
	}
	var all []string
	for _, e := range entries {
		all = append(all, e)
	}
	writeTestFile(t, dir, composite.FileName, `{"packages": {`+strings.Join(all, ",")+`}}`)
	return dir
}

// generateWithLoader runs a flagless muxt generate in dir with the real
// generate runner and the go command, counting how often packages are loaded.
func generateWithLoader(t *testing.T, dir string) (stdout string, loads int, err error) {
	t.Helper()
	counting := func(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error) {
		loads++
		return loadFromGoCommand(root, dirs, patterns)
	}
	var out bytes.Buffer
	err = commands(dir, []string{"generate"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, &bytes.Buffer{}, runners{
		generate: generateRunner(counting),
	})
	return out.String(), loads, err
}

// TestGenerateEveryMuxtJSONEntryLoadsOnce states that a flagless muxt generate
// in a muxt.json directory with entries for "." and "workspace" loads the
// packages once and writes a routes file for each directory, each naming its
// own receiver's method.
func TestGenerateEveryMuxtJSONEntryLoadsOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)

	_, loads, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	assert.Equal(t, 1, loads, "package loads")
	for path, method := range map[string]string{
		filepath.Join(dir, "template_routes.go"):              "RootPage",
		filepath.Join(dir, "workspace", "template_routes.go"): "SpacePage",
	} {
		got, err := os.ReadFile(path)
		require.NoError(t, err, "routes file")
		assert.Contains(t, string(got), method, "%s names the receiver method", path)
	}
}

// TestGenerateEveryMuxtJSONEntryNamesEachDirectory states that a run over
// several entries prints each entry's key before that directory's output,
// and a run over one entry prints no key.
func TestGenerateEveryMuxtJSONEntryNamesEachDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)

	stdout, _, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	const want = "example.com/m\nwrote template_routes.go: 1 route\nexample.com/m/workspace\nwrote template_routes.go: 1 route\n"
	assert.Equal(t, want, stdout, "stdout")

	stdout, _, err = generateWithLoader(t, filepath.Join(dir, "workspace"))

	require.NoError(t, err, "generate in workspace")
	assert.Equal(t, "wrote template_routes.go: 1 route\n", stdout, "stdout of a run over one entry")
}

// TestGenerateMuxtJSONEntriesInImportOrder states that a muxt generate over
// entries a and b, where a imports b, generates b first, though the keys sort
// as a, b.
func TestGenerateMuxtJSONEntriesInImportOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"a": "A", "b": "B"}, map[string][]string{"a": {"b"}})

	stdout, _, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	const want = "example.com/m/b\nwrote template_routes.go: 1 route\nexample.com/m/a\nwrote template_routes.go: 1 route\n"
	assert.Equal(t, want, stdout, "stdout")
}

// TestGenerateKeepsDependencyWhenDependentFails states that when package a
// imports b and a's entry names a receiver type a does not declare, a flagless
// muxt generate fails, and b's routes file is still written, since directories
// are written one at a time in import order.
func TestGenerateKeepsDependencyWhenDependentFails(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "b"), 0o755))
	writeTestFile(t, dir, "b/package.go", `package b

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type B struct{}

func (B) BPage() any { return nil }
`)
	writeTestFile(t, dir, "b/page.gohtml", `{{define "GET /B BPage()"}}<p>{{.}}</p>{{end}}`)
	writeTestFile(t, dir, "a/package.go", `package a

import (
	"embed"
	"html/template"

	_ "example.com/m/b"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type A struct{}

func (A) APage() any { return nil }
`)
	writeTestFile(t, dir, "a/page.gohtml", `{{define "GET /A APage()"}}<p>{{.}}</p>{{end}}`)
	writeTestFile(t, dir, composite.FileName, `{"packages": {"example.com/m/a": {"use_receiver_type": "Missing"}, "example.com/m/b": {"use_receiver_type": "B"}}}`)

	stdout, _, err := generateWithLoader(t, dir)

	require.Error(t, err, "generate with a receiver type a does not declare")
	const want = "example.com/m/b\nwrote template_routes.go: 1 route\nexample.com/m/a\n"
	assert.Equal(t, want, stdout, "stdout names b's output, then a, which failed")
	assert.FileExists(t, filepath.Join(dir, "b", "template_routes.go"), "b's routes file")
	assert.NoFileExists(t, filepath.Join(dir, "a", "template_routes.go"), "a's routes file")
}

// TestGenerateReloadsPackagesWhenADependencyChanged states that when package a
// imports b, a run that creates b's routes file loads the packages again
// before resolving a, and a run that finds both routes files current loads
// them once.
func TestGenerateReloadsPackagesWhenADependencyChanged(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"a": "A", "b": "B"}, map[string][]string{"a": {"b"}})

	_, loads, err := generateWithLoader(t, dir)
	require.NoError(t, err, "first generate")
	assert.Equal(t, 2, loads, "package loads when b's routes file is created")

	_, loads, err = generateWithLoader(t, dir)
	require.NoError(t, err, "second generate")
	assert.Equal(t, 1, loads, "package loads when both routes files are current")
}
