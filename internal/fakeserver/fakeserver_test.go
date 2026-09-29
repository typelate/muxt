package fakeserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				require.EqualError(t, err, tt.wantErr, "libraryPackage(%q)", tt.path)
				return
			}
			require.NoError(t, err, "libraryPackage(%q)", tt.path)
			require.Same(t, tt.want, got, "libraryPackage(%q)", tt.path)
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
			require.NoError(t, err, "renderMain()")
			for _, want := range tt.want {
				assert.Contains(t, string(got), want, "renderMain() want containing %q", want)
			}
			for _, notWant := range tt.notWant {
				assert.NotContains(t, string(got), notWant, "renderMain() want without %q", notWant)
			}
		})
	}
}

func TestRenderMainRejectsAnInvalidImportPath(t *testing.T) {
	_, err := renderMain(Config{RoutesFunction: "Routes", ReceiverInterface: "Receiver", PackagePath: "not valid \n"}, "routes")
	require.Error(t, err, "renderMain()")
	require.True(t, strings.HasPrefix(err.Error(), "formatting main.go: "), "renderMain() error = %v, want a formatting error", err)
}

func TestPreloadedCache(t *testing.T) {
	pl := []*packages.Package{{PkgPath: "example.com/a"}}
	cache := &preloadedCache{pkgPath: "example.com/a", packages: pl}

	t.Run("its package", func(t *testing.T) {
		got, ok := cache.Load("example.com/a")
		assert.True(t, ok, "Load(its package) ok")
		assert.Len(t, got, 1, "Load(its package) want the loaded packages")
	})

	t.Run("another package", func(t *testing.T) {
		got, ok := cache.Load("example.com/b")
		assert.False(t, ok, "Load(another package) ok")
		assert.Nil(t, got, "Load(another package) want nothing")
	})

	t.Run("store is ignored", func(t *testing.T) {
		cache.Store("example.com/b", nil)
		_, ok := cache.Load("example.com/b")
		assert.False(t, ok, "Store made another package loadable, want it ignored")
	})
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
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	_, pl, err := load.Packages(dir)
	require.NoError(t, err)

	t.Run("a library with a receiver interface", func(t *testing.T) {
		files, err := Generate(Config{
			PackagePath:       "example.com/app",
			PackageDir:        dir,
			RoutesFunction:    "TemplateRoutes",
			ReceiverInterface: "RoutesReceiver",
			FakeImportPath:    "example.com/app/cmd/explore/internal/fake",
		}, pl)
		require.NoError(t, err, "Generate()")
		for _, want := range []string{`routes "example.com/app"`, "routes.TemplateRoutes(mux, receiver)"} {
			assert.Contains(t, string(files.Main), want, "Main want containing %q", want)
		}
		for _, want := range []string{"package fake", "type RoutesReceiver struct", "func (fake *RoutesReceiver) Home() any"} {
			assert.Contains(t, string(files.Fake), want, "Fake want containing %q", want)
		}
	})

	t.Run("a missing package", func(t *testing.T) {
		_, err := Generate(Config{PackagePath: "example.com/missing"}, pl)
		assert.Error(t, err, "Generate(missing package)")
	})

	t.Run("a missing interface", func(t *testing.T) {
		_, err := Generate(Config{PackagePath: "example.com/app", PackageDir: dir, ReceiverInterface: "Missing"}, pl)
		assert.Error(t, err, "Generate(missing interface)")
	})
}
