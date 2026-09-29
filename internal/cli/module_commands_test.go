package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/fakeserver"
)

func TestAbsoluteDir(t *testing.T) {
	for _, tt := range []struct{ wd, dir, want string }{
		{"/work", "sub", "/work/sub"},
		{"/work", "/other", "/other"},
		{"/work", "./a/../b", "/work/b"},
	} {
		if got := absoluteDir(tt.wd, tt.dir); got != tt.want {
			t.Errorf("absoluteDir(%q, %q) = %q, want %q", tt.wd, tt.dir, got, tt.want)
		}
	}
}

func TestFakeImportPath(t *testing.T) {
	mod := &analysis.Module{ModulePath: "example.com/app", ModuleDir: "/work"}
	for _, tt := range []struct{ outDir, want string }{
		{"/work/cmd/explore-goland", "example.com/app/cmd/explore-goland/internal/fake"},
		{"/work/x", "example.com/app/x/internal/fake"},
	} {
		got, err := fakeImportPath(mod, tt.outDir)
		if err != nil || got != tt.want {
			t.Errorf("fakeImportPath(%q) = %q, %v, want %q", tt.outDir, got, err, tt.want)
		}
	}
}

func TestPackageInDirectory(t *testing.T) {
	mod := &analysis.Module{Packages: []analysis.PackageInfo{
		{Path: "example.com/a", Dir: "/work/a"},
		{Path: "example.com/b", Dir: "/work/b"},
	}}
	got, err := packageInDirectory(mod, "/work/b")
	if err != nil || got.Path != "example.com/b" {
		t.Fatalf("packageInDirectory(/work/b) = %+v, %v, want example.com/b", got, err)
	}
	_, err = packageInDirectory(mod, "/work/c")
	if want := "no muxt-generated package found at /work/c"; err == nil || err.Error() != want {
		t.Fatalf("packageInDirectory(/work/c) error = %v, want %q", err, want)
	}
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
	if got := newFakeServerConfig(pkg, "example.com/app/out/internal/fake"); !reflect.DeepEqual(got, want) {
		t.Errorf("newFakeServerConfig() = %+v, want %+v", got, want)
	}
}

func TestWriteFakeServer(t *testing.T) {
	out := filepath.Join(t.TempDir(), "explore")
	files := &fakeserver.Files{Main: []byte("package main\n"), Fake: []byte("package fake\n")}
	if err := writeFakeServer(out, files); err != nil {
		t.Fatalf("writeFakeServer() error = %v", err)
	}
	for path, want := range map[string]string{
		filepath.Join(out, "main.go"):                         "package main\n",
		filepath.Join(out, "internal", "fake", "receiver.go"): "package fake\n",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v, want %q", path, got, err, want)
		}
	}
}

func TestWriteFakeServerFailsUnderAFile(t *testing.T) {
	blocker := writeTestFile(t, t.TempDir(), "file", "")
	if err := writeFakeServer(filepath.Join(blocker, "out"), &fakeserver.Files{}); err == nil {
		t.Fatal("writeFakeServer() under a regular file = nil error, want one")
	}
}

func newTwoPackageModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com\n\ngo 1.24\n")
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
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
		if _, _, err := execute(t, wd, "-C", name, "generate", "--receiver-type=Server"); err != nil {
			t.Fatalf("generate in %s error = %v", name, err)
		}
	}

	stdout, _, err := execute(t, wd, "generate-fake-server", "a", "b", "-o", "out")
	if err != nil {
		t.Fatalf("generate-fake-server error = %v", err)
	}
	if want := "Run: go run ./out\nRun: go run ./out\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	main, err := os.ReadFile(filepath.Join(wd, "out", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `b "example.com/b"`) || strings.Contains(string(main), `"example.com/a"`) {
		t.Errorf("main.go = %s\nwant only package b, the last argument", main)
	}
}

func TestExploreModuleListsGeneratedPackages(t *testing.T) {
	wd := newTwoPackageModule(t)
	if _, _, err := execute(t, wd, "-C", "a", "generate"); err != nil {
		t.Fatalf("generate error = %v", err)
	}
	stdout, _, err := execute(t, wd, "explore-module", "--format=json")
	if err != nil {
		t.Fatalf("explore-module error = %v", err)
	}
	if !strings.Contains(stdout, `"path": "example.com/a"`) || strings.Contains(stdout, `"path": "example.com/b"`) {
		t.Errorf("explore-module output = %s\nwant only package a", stdout)
	}
}

func TestGenerateFakeServerRejectsADirectoryWithoutRoutes(t *testing.T) {
	wd := newTwoPackageModule(t)
	_, _, err := execute(t, wd, "generate-fake-server", "a")
	if want := "no muxt-generated package found at " + filepath.Join(wd, "a"); err == nil || !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("generate-fake-server error = %v, want ending %q", err, want)
	}
}
