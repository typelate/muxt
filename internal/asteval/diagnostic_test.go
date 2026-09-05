package asteval_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
)

func TestNoPackageError(t *testing.T) {
	t.Run("no packages loaded", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		err := asteval.NoPackageError(t.TempDir(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loaded no packages")
		assert.Contains(t, err.Error(), "inherits GOWORK, GOFLAGS, and GOROOT")
	})
	t.Run("load errors are forwarded", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		pl := []*packages.Package{
			{PkgPath: "fmt"},
			{PkgPath: "example.com/broken", Errors: []packages.Error{
				{Pos: "main.go:25:2", Msg: "undefined: TemplateRoutes"},
			}},
		}
		err := asteval.NoPackageError(t.TempDir(), pl)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loaded 2 packages: fmt, example.com/broken")
		assert.Contains(t, err.Error(), "undefined: TemplateRoutes")
	})
	t.Run("at most three load errors", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		pkg := &packages.Package{PkgPath: "example.com/broken"}
		for range 5 {
			pkg.Errors = append(pkg.Errors, packages.Error{Msg: "boom"})
		}
		err := asteval.NoPackageError(t.TempDir(), []*packages.Package{pkg})
		require.Error(t, err)
		assert.Equal(t, 3, strings.Count(err.Error(), "boom"))
		assert.Contains(t, err.Error(), "more load errors omitted")
	})
	t.Run("a discovered parent go.work is named", func(t *testing.T) {
		t.Setenv("GOWORK", "")
		parent := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(parent, "go.work"), []byte("go 1.24\n"), 0o600))
		dir := filepath.Join(parent, "app")
		require.NoError(t, os.Mkdir(dir, 0o700))
		err := asteval.NoPackageError(dir, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join(parent, "go.work"))
		assert.Contains(t, err.Error(), "GOWORK=off")
		assert.Contains(t, err.Error(), "go work use "+dir)
	})
	t.Run("an explicit GOWORK is named", func(t *testing.T) {
		t.Setenv("GOWORK", "/somewhere/go.work")
		err := asteval.NoPackageError(t.TempDir(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GOWORK=/somewhere/go.work is set")
	})
	t.Run("GOWORK off adds no workspace note", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		err := asteval.NoPackageError(t.TempDir(), nil)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "go work use")
	})
}
