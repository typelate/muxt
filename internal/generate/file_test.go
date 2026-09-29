package generate

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

// outputFile is a File for a package that imports nothing: what the
// import bookkeeping needs, and no more.
func outputFile() *File {
	return newFile(source.Package{Types: types.NewPackage("example.com/server", "server")})
}

func TestImports(t *testing.T) {
	genDecl := func(file *File) string {
		decl := &ast.GenDecl{Tok: token.IMPORT}
		for _, spec := range file.ImportSpecs() {
			decl.Specs = append(decl.Specs, spec)
		}
		return astgen.Format(decl)
	}
	t.Run("initial add", func(t *testing.T) {
		file := outputFile()
		assert.Equal(t, "http", file.Import("http", "net/http"))
		assert.Equal(t, `import "net/http"`, genDecl(file))
	})
	t.Run("initial with pkg ident", func(t *testing.T) {
		file := outputFile()
		assert.Equal(t, "p", file.Import("p", "net/http"))
		assert.Equal(t, `import p "net/http"`, genDecl(file))
	})
	t.Run("initial with empty ident", func(t *testing.T) {
		file := outputFile()
		assert.Equal(t, "http", file.Import("", "net/http"))
		assert.Equal(t, `import "net/http"`, genDecl(file))
	})
	t.Run("imports are listed sorted by path", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("", "net/http")
		_ = file.Import("", "html/template")
		assert.Equal(t, `import (
	"html/template"
	"net/http"
)`, genDecl(file))
	})
	t.Run("it respects order", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("", "html/template")
		_ = file.Import("", "net/http")
		assert.Equal(t, `import (
	"html/template"
	"net/http"
)`, genDecl(file))
	})
	t.Run("it returns the registered identifier", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("t", "html/template")
		assert.Equal(t, "t", file.Import("", "html/template"))
	})
	t.Run("it returns the package path base", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("", "html/template")
		assert.Equal(t, "template", file.Import("", "html/template"))
	})
}

func TestHTTPStatusCode(t *testing.T) {
	file := outputFile()

	exp := astgen.HTTPStatusCode(file, 600)
	require.NotNil(t, exp)
	lit, ok := exp.(*ast.BasicLit)
	require.True(t, ok, "HTTPStatusCode(600) is a %T, want a literal", exp)
	assert.Equal(t, token.INT, lit.Kind)
	assert.Equal(t, "600", lit.Value)
	assert.Empty(t, file.ImportSpecs(), "it should not add the import if it is not needed")
}
