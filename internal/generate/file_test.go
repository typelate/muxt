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
		assert.Equal(t, genDecl(file), `import "net/http"`)
	})
	t.Run("initial with pkg ident", func(t *testing.T) {
		file := outputFile()
		assert.Equal(t, "p", file.Import("p", "net/http"))
		assert.Equal(t, genDecl(file), `import p "net/http"`)
	})
	t.Run("initial with empty ident", func(t *testing.T) {
		file := outputFile()
		assert.Equal(t, "http", file.Import("", "net/http"))
		assert.Equal(t, genDecl(file), `import "net/http"`)
	})
	t.Run("initial with empty ident", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("", "net/http")
		_ = file.Import("", "html/template")
		assert.Equal(t, genDecl(file), `import (
	"html/template"
	"net/http"
)`)
	})
	t.Run("it respects order", func(t *testing.T) {
		file := outputFile()
		_ = file.Import("", "html/template")
		_ = file.Import("", "net/http")
		assert.Equal(t, genDecl(file), `import (
	"html/template"
	"net/http"
)`)
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
	require.True(t, ok)
	require.Equal(t, token.INT, lit.Kind)
	require.Equal(t, "600", lit.Value)
	require.Empty(t, file.ImportSpecs(), "it should not add the import if it is not needed")
}
