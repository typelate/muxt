package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/load"
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
func generateWithLoader(t *testing.T, dir string, flags ...string) (stdout string, loads int, err error) {
	t.Helper()
	counting := func(root string, dirs, patterns []string) (*load.Loaded, error) {
		loads++
		return loadFromGoCommand(root, dirs, patterns)
	}
	var out bytes.Buffer
	err = commands(dir, append([]string{"generate"}, flags...), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, &bytes.Buffer{}, runners{
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

// TestGenerateEveryMuxtJSONEntryNamesEachDirectory states that with -v a run
// over several entries prints each entry's key before that directory's
// output, and a run over one entry prints no key.
func TestGenerateEveryMuxtJSONEntryNamesEachDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)

	stdout, _, err := generateWithLoader(t, dir, "-v")

	require.NoError(t, err, "generate -v")
	const want = "example.com/m\ngenerating handler for pattern GET /Root\nwrote template_routes.go: 1 route\nexample.com/m/workspace\ngenerating handler for pattern GET /Space\nwrote template_routes.go: 1 route\n"
	assert.Equal(t, want, stdout, "stdout")

	stdout, _, err = generateWithLoader(t, filepath.Join(dir, "workspace"), "-v")

	require.NoError(t, err, "generate -v in workspace")
	assert.Equal(t, "generating handler for pattern GET /Space\nwrote template_routes.go: 1 route\n", stdout, "stdout of a run over one entry")
}

// TestGenerateSucceedsQuietly states that without -v a run that generates
// without problems prints nothing, over one entry and over several.
func TestGenerateSucceedsQuietly(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)

	stdout, _, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	assert.Empty(t, stdout, "stdout of a run over several entries")

	stdout, _, err = generateWithLoader(t, filepath.Join(dir, "workspace"))

	require.NoError(t, err, "generate in workspace")
	assert.Empty(t, stdout, "stdout of a run over one entry")
}

// TestGenerateMuxtJSONEntriesInImportOrder states that a muxt generate over
// entries a and b, where a imports b, generates b first, though the keys sort
// as a, b.
func TestGenerateMuxtJSONEntriesInImportOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"a": "A", "b": "B"}, map[string][]string{"a": {"b"}})

	stdout, _, err := generateWithLoader(t, dir, "-v")

	require.NoError(t, err, "generate -v")
	const want = "example.com/m/b\ngenerating handler for pattern GET /B\nwrote template_routes.go: 1 route\nexample.com/m/a\ngenerating handler for pattern GET /A\nwrote template_routes.go: 1 route\n"
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
	assert.Empty(t, stdout, "stdout")
	assert.ErrorContains(t, err, "example.com/m/a: ", "the error names the package that failed")
	assert.NotContains(t, err.Error(), "example.com/m/b", "the error names only the failed package")
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

// TestGenerateChecksEveryPackageLoadedBeforeWriting states that when the
// package of a later muxt.json entry has no Go files, a flagless muxt generate
// fails naming the file and that entry, and writes nothing for the entries
// before it.
func TestGenerateChecksEveryPackageLoadedBeforeWriting(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"a": "A"}, nil)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "empty"), 0o755))
	writeTestFile(t, dir, composite.FileName, `{"packages": {"example.com/m/a": {"use_receiver_type": "A"}, "example.com/m/empty": {}}}`)

	_, _, err := generateWithLoader(t, dir)

	var invalid *composite.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, filepath.Join(dir, composite.FileName), invalid.Path, "path")
	assert.Equal(t, `packages["example.com/m/empty"]`, invalid.Where, "entry")
	want := filepath.Join(dir, composite.FileName) + `: packages["example.com/m/empty"]: package example.com/m/empty at ` + filepath.Join(dir, "empty") + ` has no Go files for this build`
	assert.Equal(t, want, err.Error(), "message")
	assert.NoFileExists(t, filepath.Join(dir, "a", "template_routes.go"), "an earlier entry's routes file")
}

// notedModule writes a module with muxt.json entries for packages a and b,
// each with receiver type S and a route calling Missing, a method S lacks.
func notedModule(t *testing.T, keys ...string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	var entries []string
	for _, key := range keys {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, key), 0o755))
		writeTestFile(t, dir, key+"/package.go", fmt.Sprintf(`package %s

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type S struct{}
`, key))
		writeTestFile(t, dir, key+"/page.gohtml", `{{define "GET /{id} Missing(ctx, id)"}}<p>{{.}}</p>{{end}}`)
		entries = append(entries, fmt.Sprintf(`"example.com/m/%s": {"use_receiver_type": "S"}`, key))
	}
	writeTestFile(t, dir, composite.FileName, `{"packages": {`+strings.Join(entries, ",")+`}}`)
	return dir
}

// TestGenerateNotesNameTheirEntry states that when a run covers several
// entries, each note a job prints starts with that entry's key, and a run over
// one entry prints its notes bare.
func TestGenerateNotesNameTheirEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := notedModule(t, "a", "b")

	stdout, _, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	for _, key := range []string{"example.com/m/a", "example.com/m/b"} {
		assert.Contains(t, stdout, key+": note: S does not define Missing(", "notes of %s", key)
	}
	for line := range strings.Lines(stdout) {
		assert.True(t, strings.HasPrefix(line, "example.com/m/a: ") || strings.HasPrefix(line, "example.com/m/b: "), "line without a key: %q", line)
	}

	stdout, _, err = generateWithLoader(t, filepath.Join(dir, "a"))

	require.NoError(t, err, "generate in a")
	assert.True(t, strings.HasPrefix(stdout, "note: S does not define Missing("), "stdout of a run over one entry:\n%s", stdout)
}

// TestGenerateOfOnePackagePrintsItsLoadDiagnostic states that a run of one
// package whose directory has no Go files, from flags or from a single
// muxt.json entry, prints the multi-line diagnostic to stderr the way a run
// did before generate checked packages up front, and returns the error
// unwrapped.
func TestGenerateOfOnePackagePrintsItsLoadDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name  string
		entry string
		flags []string
	}{
		{name: "flags", flags: []string{"--use-receiver-type=S"}},
		{name: "one entry", entry: `{"packages": {"example.com/m": {"use_receiver_type": "S"}}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
			if tt.entry != "" {
				writeTestFile(t, dir, composite.FileName, tt.entry)
			}
			var stdout, stderr bytes.Buffer

			err := commands(dir, append([]string{"generate"}, tt.flags...), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &stdout, &stderr, runners{
				generate: generateRunner(loadFromGoCommand),
			})

			var lookup *load.PackageLookupError
			require.ErrorAs(t, err, &lookup, "error %v", err)
			assert.Equal(t, "Error:\n"+lookup.MultiLineError()+"\n", stderr.String(), "stderr")
			assert.Contains(t, stderr.String(), "\tmuxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT", "stderr hint")
			assert.Empty(t, stdout.String(), "stdout")
		})
	}
}
