package generate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
}
