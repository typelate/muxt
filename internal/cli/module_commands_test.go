package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
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
	} {
		got, err := fakeImportPath(mod, tt.outDir)
		assert.NoError(t, err, "fakeImportPath(%q)", tt.outDir)
		assert.Equal(t, tt.want, got, "fakeImportPath(%q)", tt.outDir)
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

// generate-fake-server takes several package directories but writes one
// main.go and one receiver.go, so each package overwrites the one before.
// This test records that; the command's help does not say which is intended.
func TestGenerateFakeServerWithSeveralPackagesKeepsTheLast(t *testing.T) {
	wd := newTwoPackageModule(t)
	for _, name := range []string{"a", "b"} {
		_, _, err := execute(t, wd, "-C", name, "generate", "--receiver-type=Server")
		require.NoError(t, err, "generate in %s", name)
	}

	stdout, _, err := execute(t, wd, "generate-fake-server", "a", "b", "-o", "out")
	require.NoError(t, err, "generate-fake-server")
	assert.Equal(t, "Run: go run ./out\nRun: go run ./out\n", stdout, "stdout")
	main, err := os.ReadFile(filepath.Join(wd, "out", "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(main), `b "example.com/b"`, "main.go want package b, the last argument")
	assert.NotContains(t, string(main), `"example.com/a"`, "main.go want only package b, the last argument")
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
