package generate

import (
	"go/token"
	"go/types"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/source"
)

func TestFileImport(t *testing.T) {
	t.Run("a path is named for its last element and keeps that name", func(t *testing.T) {
		file := scalarTestFile(t)
		assert.Equal(t, "http", file.Import("", "net/http"), "Import(net/http)")
		assert.Equal(t, "http", file.Import("", "net/http"), "Import(net/http) again")
	})

	t.Run("a name another path holds gets a 12 character hash and keeps it", func(t *testing.T) {
		file := scalarTestFile(t)
		file.Import("", "net/http")

		aliased := file.Import("", "example.com/other/http")
		assert.True(t, strings.HasPrefix(aliased, "http"), "Import(example.com/other/http) = %q, want it to start with http", aliased)
		assert.NotEqual(t, "http", aliased, "Import(example.com/other/http)")
		assert.Len(t, aliased, len("http")+12, "Import(example.com/other/http) = %q, want http followed by a 12 character hash", aliased)
		assert.Equal(t, aliased, file.Import("", "example.com/other/http"), "Import(example.com/other/http) again")
	})

	t.Run("ImportSpecs lists the paths sorted", func(t *testing.T) {
		file := scalarTestFile(t)
		file.Import("", "net/http")
		file.Import("", "example.com/other/http")

		var paths []string
		for _, spec := range file.ImportSpecs() {
			paths = append(paths, spec.Path.Value)
		}
		assert.Equal(t, []string{`"example.com/other/http"`, `"net/http"`}, paths, "ImportSpecs paths")
	})

	// A generated handler declares request, td, err and the like, and
	// spells types from imported packages inside it; the output package
	// declares the templates variable at package scope, which an import
	// name may not repeat. A package with such a name is aliased.
	for _, importPath := range []string{
		"example.com/request",
		"example.com/response",
		"example.com/receiver",
		"example.com/td",
		"example.com/err",
		"example.com/ctx",
		"example.com/flusher",
		"example.com/len",
		"example.com/templates",
	} {
		t.Run("a path named like a name in scope where its types are spelled gets a hash: "+importPath, func(t *testing.T) {
			pkg := types.NewPackage("example.com/server", "server")
			pkg.Scope().Insert(types.NewVar(token.NoPos, pkg, "templates", types.Typ[types.Int]))
			file := newFile(source.Package{Types: pkg})
			name := path.Base(importPath)

			aliased := file.Import("", importPath)
			assert.True(t, strings.HasPrefix(aliased, name), "Import(%s) = %q, want it to start with %s", importPath, aliased, name)
			assert.Len(t, aliased, len(name)+12, "Import(%s) = %q, want %s followed by a 12 character hash", importPath, aliased, name)
		})
	}

	t.Run("a standard library path keeps its name", func(t *testing.T) {
		file := outputFile()
		for _, importPath := range []string{"net/http", "path", "net/url", "strings", "context", "log/slog"} {
			assert.Equal(t, path.Base(importPath), file.Import("", importPath), "Import(%s)", importPath)
		}
	})

	t.Run("importing the output package is a generator bug, not a fatal exit", func(t *testing.T) {
		file := outputFile()
		assert.PanicsWithValue(t, "generate: a generated file cannot import its own package example.com/server", func() {
			file.Import("", "example.com/server")
		}, "Import of the output package")
	})
}
