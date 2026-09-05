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
		dir := t.TempDir()
		err := asteval.NoPackageError(dir, nil)
		require.Error(t, err)
		require.Equal(t, "no Go package found at "+dir, err.Error(), "the short form is a single line")
		assert.Contains(t, multiLine(t, err), "loaded no packages")
		assert.Contains(t, multiLine(t, err), "inherits GOWORK, GOFLAGS, and GOROOT")
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
		assert.Contains(t, multiLine(t, err), "loaded 2 packages: fmt, example.com/broken")
		assert.Contains(t, multiLine(t, err), "undefined: TemplateRoutes")
	})
	t.Run("at most three load errors", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		pkg := &packages.Package{PkgPath: "example.com/broken"}
		for range 5 {
			pkg.Errors = append(pkg.Errors, packages.Error{Msg: "boom"})
		}
		err := asteval.NoPackageError(t.TempDir(), []*packages.Package{pkg})
		require.Error(t, err)
		assert.Equal(t, 3, strings.Count(multiLine(t, err), "boom"))
		assert.Contains(t, multiLine(t, err), "more load errors omitted")
	})
	t.Run("a discovered parent go.work is named", func(t *testing.T) {
		t.Setenv("GOWORK", "")
		parent := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(parent, "go.work"), []byte("go 1.24\n"), 0o600))
		dir := filepath.Join(parent, "app")
		require.NoError(t, os.Mkdir(dir, 0o700))
		err := asteval.NoPackageError(dir, nil)
		require.Error(t, err)
		assert.Contains(t, multiLine(t, err), filepath.Join(parent, "go.work"))
		assert.Contains(t, multiLine(t, err), "GOWORK=off")
		assert.Contains(t, multiLine(t, err), "go work use "+dir)
	})
	t.Run("GOWORK auto discovers a parent go.work", func(t *testing.T) {
		t.Setenv("GOWORK", "auto")
		parent := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(parent, "go.work"), []byte("go 1.24\n"), 0o600))
		dir := filepath.Join(parent, "app")
		require.NoError(t, os.Mkdir(dir, 0o700))
		err := asteval.NoPackageError(dir, nil)
		require.Error(t, err)
		assert.Contains(t, multiLine(t, err), filepath.Join(parent, "go.work"))
		assert.NotContains(t, multiLine(t, err), "GOWORK=auto is set")
	})
	t.Run("an explicit GOWORK is named", func(t *testing.T) {
		t.Setenv("GOWORK", "/somewhere/go.work")
		err := asteval.NoPackageError(t.TempDir(), nil)
		require.Error(t, err)
		assert.Contains(t, multiLine(t, err), "GOWORK=/somewhere/go.work is set")
	})
	t.Run("GOWORK off adds no workspace note", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		err := asteval.NoPackageError(t.TempDir(), nil)
		require.Error(t, err)
		assert.NotContains(t, multiLine(t, err), "go work use")
	})
}

// multiLine returns err's verbose rendering, failing when err has none.
func multiLine(t *testing.T, err error) string {
	t.Helper()
	e, ok := err.(interface{ MultiLineError() string })
	require.True(t, ok, "error should have a multi-line form")
	return e.MultiLineError()
}
