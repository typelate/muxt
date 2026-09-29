package fakeserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/load"
)

func TestLibraryPackage(t *testing.T) {
	lib := &packages.Package{PkgPath: "example.com/lib", Name: "lib"}
	other := &packages.Package{PkgPath: "example.com/other", Name: "other"}
	cmd := &packages.Package{PkgPath: "example.com/cmd", Name: "main"}
	pl := []*packages.Package{other, lib, cmd}

	for _, tt := range []struct {
		name    string
		path    string
		want    *packages.Package
		wantErr string
	}{
		{name: "a library", path: "example.com/lib", want: lib},
		{name: "not loaded", path: "example.com/missing", wantErr: `package "example.com/missing" not found in loaded packages`},
		{name: "package main", path: "example.com/cmd", wantErr: `cannot generate fake server for package "example.com/cmd": package is main (the target package must be a library)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := libraryPackage(pl, tt.path)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("libraryPackage(%q) error = %v, want %q", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("libraryPackage(%q) = %v, %v, want %v", tt.path, got, err, tt.want)
			}
		})
	}
}

func TestRenderMain(t *testing.T) {
	config := Config{
		PackagePath:       "example.com/app/routes",
		RoutesFunction:    "Routes",
		ReceiverInterface: "Receiver",
		FakeImportPath:    "example.com/app/cmd/explore/internal/fake",
	}
	for _, tt := range []struct {
		name        string
		change      func(*Config)
		packageName string
		want        []string
		notWant     []string
	}{
		{
			name:        "the plain call",
			packageName: "routes",
			want: []string{
				`routes "example.com/app/routes"`,
				`"example.com/app/cmd/explore/internal/fake"`,
				`receiver := new(fake.Receiver)`,
				`routes.Routes(mux, receiver)`,
			},
			notWant: []string{"slog.Default())", `, "")`, ", nil)"},
		},
		{
			name:        "every optional parameter",
			change:      func(c *Config) { c.Logger, c.PathPrefix, c.Middleware = true, true, true },
			packageName: "routes",
			want:        []string{`routes.Routes(mux, receiver, slog.Default(), "", nil)`},
		},
		{
			name:        "a logger only",
			change:      func(c *Config) { c.Logger = true },
			packageName: "routes",
			want:        []string{`routes.Routes(mux, receiver, slog.Default())`},
		},
		{
			name:        "a path prefix only",
			change:      func(c *Config) { c.PathPrefix = true },
			packageName: "routes",
			want:        []string{`routes.Routes(mux, receiver, "")`},
		},
		{
			name:        "middleware only",
			change:      func(c *Config) { c.Middleware = true },
			packageName: "routes",
			want:        []string{`routes.Routes(mux, receiver, nil)`},
		},
		{
			name:        "a package named like an import of main",
			packageName: "http",
			want:        []string{`httppkg "example.com/app/routes"`, `httppkg.Routes(mux, receiver)`},
		},
		{
			name:        "a package named fake",
			packageName: "fake",
			want:        []string{`fakepkg "example.com/app/routes"`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := config
			if tt.change != nil {
				tt.change(&c)
			}
			got, err := renderMain(c, tt.packageName)
			if err != nil {
				t.Fatalf("renderMain() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(got), want) {
					t.Errorf("renderMain() = %s\nwant containing %q", got, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(string(got), notWant) {
					t.Errorf("renderMain() = %s\nwant without %q", got, notWant)
				}
			}
		})
	}
}

func TestRenderMainRejectsAnInvalidImportPath(t *testing.T) {
	_, err := renderMain(Config{RoutesFunction: "Routes", ReceiverInterface: "Receiver", PackagePath: "not valid \n"}, "routes")
	if err == nil || !strings.HasPrefix(err.Error(), "formatting main.go: ") {
		t.Fatalf("renderMain() error = %v, want a formatting error", err)
	}
}

func TestPreloadedCache(t *testing.T) {
	pl := []*packages.Package{{PkgPath: "example.com/a"}}
	cache := &preloadedCache{pkgPath: "example.com/a", packages: pl}

	if got, ok := cache.Load("example.com/a"); !ok || len(got) != 1 {
		t.Errorf("Load(its package) = %v, %v, want the loaded packages", got, ok)
	}
	if got, ok := cache.Load("example.com/b"); ok || got != nil {
		t.Errorf("Load(another package) = %v, %v, want nothing", got, ok)
	}
	cache.Store("example.com/b", nil)
	if _, ok := cache.Load("example.com/b"); ok {
		t.Error("Store made another package loadable, want it ignored")
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.24\n",
		"routes.go": `package routes

import "net/http"

type RoutesReceiver interface {
	Home() any
}

func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver) {}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, pl, err := load.Packages(dir)
	if err != nil {
		t.Fatal(err)
	}

	files, err := Generate(Config{
		PackagePath:       "example.com/app",
		PackageDir:        dir,
		RoutesFunction:    "TemplateRoutes",
		ReceiverInterface: "RoutesReceiver",
		FakeImportPath:    "example.com/app/cmd/explore/internal/fake",
	}, pl)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, want := range []string{`routes "example.com/app"`, "routes.TemplateRoutes(mux, receiver)"} {
		if !strings.Contains(string(files.Main), want) {
			t.Errorf("Main = %s\nwant containing %q", files.Main, want)
		}
	}
	for _, want := range []string{"package fake", "type RoutesReceiver struct", "func (fake *RoutesReceiver) Home() any"} {
		if !strings.Contains(string(files.Fake), want) {
			t.Errorf("Fake = %s\nwant containing %q", files.Fake, want)
		}
	}

	if _, err := Generate(Config{PackagePath: "example.com/missing"}, pl); err == nil {
		t.Error("Generate(missing package) = nil error, want one")
	}
	if _, err := Generate(Config{PackagePath: "example.com/app", PackageDir: dir, ReceiverInterface: "Missing"}, pl); err == nil {
		t.Error("Generate(missing interface) = nil error, want one")
	}
}
