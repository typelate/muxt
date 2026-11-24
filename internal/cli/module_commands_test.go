package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/fakeserver"
)

func TestAbsoluteDir(t *testing.T) {
	for _, tt := range []struct{ wd, dir, want string }{
		{"/work", "sub", "/work/sub"},
		{"/work", "/other", "/other"},
		{"/work", "./a/../b", "/work/b"},
	} {
		assert.Equal(t, tt.want, absoluteDir(tt.wd, tt.dir), "absoluteDir(%q, %q)", tt.wd, tt.dir)
	}
}

func TestFakeImportPath(t *testing.T) {
	mod := &analysis.Module{ModulePath: "example.com/app", ModuleDir: "/work"}
	for _, tt := range []struct{ outDir, want string }{
		{"/work/cmd/explore-goland", "example.com/app/cmd/explore-goland/internal/fake"},
		{"/work/x", "example.com/app/x/internal/fake"},
		{"/work", "example.com/app/internal/fake"},
		{"/work/", "example.com/app/internal/fake"},
	} {
		got, err := fakeImportPath(mod, tt.outDir)
		assert.NoError(t, err, "fakeImportPath(%q)", tt.outDir)
		assert.Equal(t, tt.want, got, "fakeImportPath(%q)", tt.outDir)
	}
	for _, outDir := range []string{"/elsewhere", "/work-other/x", "/"} {
		_, err := fakeImportPath(mod, outDir)
		assert.EqualError(t, err, "-o "+outDir+" is outside the module at /work: the fake server must be in the module to import its package", "fakeImportPath(%q)", outDir)
	}
}

func TestPackageInDirectory(t *testing.T) {
	mod := &analysis.Module{Packages: []analysis.PackageInfo{
		{Path: "example.com/a", Dir: "/work/a"},
		{Path: "example.com/b", Dir: "/work/b"},
	}}

	t.Run("a generated package", func(t *testing.T) {
		got, err := packageInDirectory(mod, "/work/b")
		require.NoError(t, err, "packageInDirectory(/work/b)")
		assert.Equal(t, "example.com/b", got.Path, "packageInDirectory(/work/b)")
	})

	t.Run("no generated package", func(t *testing.T) {
		_, err := packageInDirectory(mod, "/work/c")
		require.EqualError(t, err, "no muxt-generated package found at /work/c", "packageInDirectory(/work/c)")
	})

	t.Run("several route sets", func(t *testing.T) {
		sets := &analysis.Module{Packages: []analysis.PackageInfo{
			{Path: "example.com/a", Dir: "/work/a", Config: analysis.PackageConfig{RoutesFunction: "PublicRoutes"}},
			{Path: "example.com/a", Dir: "/work/a", Config: analysis.PackageConfig{RoutesFunction: "AdminRoutes"}},
			{Path: "example.com/b", Dir: "/work/b"},
		}}

		_, err := packageInDirectory(sets, "/work/a")

		require.EqualError(t, err, "/work/a has 2 route sets (PublicRoutes, AdminRoutes); generate-fake-server serves one routes function and has no way to choose", "packageInDirectory(/work/a)")
		var several *SeveralRouteSetsError
		require.ErrorAs(t, err, &several, "error type")
		assert.Equal(t, []string{"PublicRoutes", "AdminRoutes"}, several.RoutesFunctions, "routes functions")
	})
}

func TestNewFakeServerConfig(t *testing.T) {
	pkg := analysis.PackageInfo{
		Path: "example.com/a",
		Dir:  "/work/a",
		Config: analysis.PackageConfig{
			RoutesFunction:    "Routes",
			ReceiverInterface: "Receiver",
			Logger:            true,
			PathPrefix:        true,
			Middleware:        true,
		},
	}
	want := fakeserver.Config{
		PackagePath:       "example.com/a",
		PackageDir:        "/work/a",
		RoutesFunction:    "Routes",
		ReceiverInterface: "Receiver",
		Logger:            true,
		PathPrefix:        true,
		Middleware:        true,
		FakeImportPath:    "example.com/app/out/internal/fake",
	}
	assert.Equal(t, want, newFakeServerConfig(pkg, "example.com/app/out/internal/fake"), "newFakeServerConfig()")
}

func TestWriteFakeServer(t *testing.T) {
	out := filepath.Join(t.TempDir(), "explore")
	files := &fakeserver.Files{Main: []byte("package main\n"), Fake: []byte("package fake\n")}
	require.NoError(t, writeFakeServer(out, files), "writeFakeServer()")
	for path, want := range map[string]string{
		filepath.Join(out, "main.go"):                         "package main\n",
		filepath.Join(out, "internal", "fake", "receiver.go"): "package fake\n",
	} {
		got, err := os.ReadFile(path)
		assert.NoError(t, err, path)
		assert.Equal(t, want, string(got), path)
	}
}

// TestWriteFakeServerRefusesASymlinkOutOfItsDirectory states that the fake
// server's files get the protection generated files have: a link that leaves
// the directory is not written through.
func TestWriteFakeServerRefusesASymlinkOutOfItsDirectory(t *testing.T) {
	victim := writeTestFile(t, t.TempDir(), "victim.txt", "keep\n")
	out := t.TempDir()
	require.NoError(t, os.Symlink(victim, filepath.Join(out, "main.go")))

	err := writeFakeServer(out, &fakeserver.Files{Main: []byte("package main\n"), Fake: []byte("package fake\n")})
	require.Error(t, err, "writeFakeServer() through a link out of the directory")

	got, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	assert.Equal(t, "keep\n", string(got), "the file the link points at")
}

func TestWriteFakeServerFailsUnderAFile(t *testing.T) {
	blocker := writeTestFile(t, t.TempDir(), "file", "")
	err := writeFakeServer(filepath.Join(blocker, "out"), &fakeserver.Files{})
	require.Error(t, err, "writeFakeServer() under a regular file")
}

func newTwoPackageModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com\n\ngo 1.24\n")
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o755))
		writeTestFile(t, filepath.Join(dir, name), "template.go", `package `+name+`

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type Server struct{}

func (s *Server) Home() any { return nil }
`)
		writeTestFile(t, filepath.Join(dir, name), "page.gohtml", `{{define "GET / Home()"}}<h1>`+name+`</h1>{{end}}`)
	}
	return dir
}

// generate-fake-server writes one main.go and one receiver.go, so it serves
// one package: a second package directory would overwrite the first.
func TestGenerateFakeServerRejectsSeveralPackages(t *testing.T) {
	wd := t.TempDir()
	_, _, err := execute(t, wd, "generate-fake-server", "a", "b", "-o", "out")
	require.EqualError(t, err, "generate-fake-server takes one package directory, got 2: a b", "generate-fake-server a b")
	assert.NoDirExists(t, filepath.Join(wd, "out"), "generate-fake-server a b wrote output")
}

func TestExploreModuleListsGeneratedPackages(t *testing.T) {
	wd := newTwoPackageModule(t)
	_, _, err := execute(t, wd, "-C", "a", "generate")
	require.NoError(t, err, "generate")
	stdout, _, err := execute(t, wd, "explore-module", "--format=json")
	require.NoError(t, err, "explore-module")
	assert.Contains(t, stdout, `"path": "example.com/a"`, "explore-module output want package a")
	assert.NotContains(t, stdout, `"path": "example.com/b"`, "explore-module output want only package a")
}

func TestGenerateFakeServerRejectsADirectoryWithoutRoutes(t *testing.T) {
	wd := newTwoPackageModule(t)
	_, _, err := execute(t, wd, "generate-fake-server", "a")
	require.Error(t, err, "generate-fake-server")
	want := "no muxt-generated package found at " + filepath.Join(wd, "a")
	require.True(t, strings.HasSuffix(err.Error(), want), "generate-fake-server error = %v, want ending %q", err, want)
}

// TestExploreModuleGenerateCommandForConfiguredPackage states that the
// generate command explore-module prints runs: flags are rejected in a
// directory a muxt.json entry configures, so that package gets the flagless
// command, and a package without an entry keeps the flags its header records.
func TestExploreModuleGenerateCommandForConfiguredPackage(t *testing.T) {
	wd := newTwoPackageModule(t)
	for _, name := range []string{"a", "b"} {
		_, _, err := execute(t, wd, "-C", name, "generate", "--output-routes-func-with-logger-param")
		require.NoError(t, err, "generate %s", name)
	}
	writeTestFile(t, wd, "muxt.json", `{"packages": {"example.com/a": {"output_routes_func_with_logger_param": true}}}`)

	stdout, _, err := execute(t, wd, "explore-module", "--format=json")
	require.NoError(t, err, "explore-module")

	var got struct {
		Packages []struct {
			Path     string
			Dir      string
			Commands struct{ Generate string }
		}
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "explore-module output")
	require.Len(t, got.Packages, 2, "explore-module packages")
	for _, pkg := range got.Packages {
		want := "muxt -C " + pkg.Dir + " generate"
		if pkg.Path == "example.com/b" {
			want += " --output-routes-func-with-logger-param"
		}
		assert.Equal(t, want, pkg.Commands.Generate, "generate command for %s", pkg.Path)
	}
}

// brokenEntryModule is a module whose package a has a generated file and a
// muxt.json entry that sets two frontends, which generate rejects.
func brokenEntryModule(t *testing.T) string {
	t.Helper()
	wd := newTwoPackageModule(t)
	_, _, err := execute(t, wd, "-C", "a", "generate")
	require.NoError(t, err, "generate")
	writeTestFile(t, wd, "muxt.json", `{"packages": {"example.com/a": {"output_htmx": true, "output_datastar": true}}}`)
	return wd
}

func requireInvalidEntry(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err, "the command read an entry that breaks a rule")
	invalid, ok := errors.AsType[*composite.InvalidEntryError](err)
	require.True(t, ok, "error = %v, want *composite.InvalidEntryError", err)
	assert.Equal(t, `packages["example.com/a"][0]`, invalid.Where, "the entry named")
	assert.True(t, strings.HasPrefix(err.Error(), invalid.Path+": packages[\"example.com/a\"][0]: "), "error = %q, want the file and the entry first", err.Error())
}

// TestExploreModuleRejectsInvalidEntry states that a package whose muxt.json
// entry breaks a rule fails explore-module as it fails every other command in
// that directory.
func TestExploreModuleRejectsInvalidEntry(t *testing.T) {
	wd := brokenEntryModule(t)
	_, _, err := execute(t, wd, "explore-module", "--format=json")
	requireInvalidEntry(t, err)
}

// TestExploreModuleRejectsTwoEntriesWithOneOutputFile states that explore-module
// fails on two entries of a key that write one file, with the error every
// command gives, since it reads the entries through the same selection.
func TestExploreModuleRejectsTwoEntriesWithOneOutputFile(t *testing.T) {
	wd := newTwoPackageModule(t)
	_, _, err := execute(t, wd, "-C", "a", "generate")
	require.NoError(t, err, "generate")
	writeTestFile(t, wd, "muxt.json", `{"packages": {"example.com/a": [
		{"output_file": "routes.go", "output_routes_func": "PublicRoutes"},
		{"output_file": "routes.go", "output_routes_func": "AdminRoutes"}
	]}}`)

	_, _, err = execute(t, wd, "explore-module", "--format=json")

	clash, ok := errors.AsType[*composite.SetClashError](err)
	require.True(t, ok, "error = %v, want *composite.SetClashError", err)
	want := composite.SetClashError{Path: filepath.Join(wd, composite.FileName), Key: "example.com/a", Member: "output_file", Value: "routes.go", First: 0, Second: 1}
	assert.Equal(t, want, *clash, "error")
}

// TestGenerateFakeServerRejectsInvalidEntry is the same for generate-fake-server.
func TestGenerateFakeServerRejectsInvalidEntry(t *testing.T) {
	wd := brokenEntryModule(t)
	_, _, err := execute(t, wd, "generate-fake-server", "a")
	requireInvalidEntry(t, err)
	assert.NoDirExists(t, filepath.Join(wd, "cmd", "explore-goland"), "generate-fake-server wrote output")
}

// TestExploreModuleListsEachSetOfADirectory states that explore-module lists
// a package with two generated route sets once per set, each with its own
// routes function, since the one directory holds two generated files that
// call different functions; a set that was never generated has no row.
func TestExploreModuleListsEachSetOfADirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	root := twoSetModule(t)
	_, _, err := execute(t, root, "generate")
	require.NoError(t, err, "generate")

	stdout, _, err := execute(t, root, "explore-module", "--format=json")
	require.NoError(t, err, "explore-module")

	var got struct {
		Packages []struct {
			Path   string
			Config struct{ RoutesFunction string }
		}
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "explore-module output")
	var rows []string
	for _, pkg := range got.Packages {
		rows = append(rows, pkg.Path+" "+pkg.Config.RoutesFunction)
	}
	assert.Equal(t, []string{testModule + " AdminRoutes", testModule + " PublicRoutes"}, rows, "explore-module rows")
}

// TestGenerateFakeServerRefusesADirectoryWithSeveralRouteSets states that
// generate-fake-server in a package with two generated route sets fails
// naming each routes function and writes nothing: its server calls one, and
// no flag says which.
func TestGenerateFakeServerRefusesADirectoryWithSeveralRouteSets(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	root := twoSetModule(t)
	_, _, err := execute(t, root, "generate")
	require.NoError(t, err, "generate")

	_, _, err = execute(t, root, "generate-fake-server", "-o", "out")

	require.Error(t, err, "generate-fake-server")
	var several *SeveralRouteSetsError
	require.ErrorAs(t, err, &several, "error %v", err)
	assert.Equal(t, []string{"AdminRoutes", "PublicRoutes"}, several.RoutesFunctions, "routes functions")
	assert.True(t, strings.HasSuffix(several.Dir, filepath.Base(root)), "directory %q is the module's %q", several.Dir, root)
	assert.NoDirExists(t, filepath.Join(root, "out"), "generate-fake-server wrote output")
}
