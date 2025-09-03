package source_test

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/source"
)

var (
	workingDir = sync.OnceValues(func() (string, error) {
		return os.Getwd()
	})
	fileSet = sync.OnceValue(func() *token.FileSet {
		return token.NewFileSet()
	})
	loadPkg = sync.OnceValues(func() ([]*packages.Package, error) {
		wd, err := workingDir()
		if err != nil {
			return nil, err
		}
		return loadPackages(wd, []string{"context", "net/http", wd})
	})
)

func loadPackages(wd string, patterns []string) ([]*packages.Package, error) {
	return packages.Load(&packages.Config{
		Fset: fileSet(),
		Mode: packages.NeedModule | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
}

func TestImports(t *testing.T) {
	genDecl := func(file *source.File) string {
		decl := &ast.GenDecl{Tok: token.IMPORT}
		for _, spec := range file.ImportSpecs() {
			decl.Specs = append(decl.Specs, spec)
		}
		return source.Format(decl)
	}
	t.Run("initial add", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		assert.Equal(t, "http", file.Import("http", "net/http"))
		assert.Equal(t, genDecl(file), `import "net/http"`)
	})
	t.Run("initial with pkg ident", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		assert.Equal(t, "p", file.Import("p", "net/http"))
		assert.Equal(t, genDecl(file), `import p "net/http"`)
	})
	t.Run("initial with empty ident", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		assert.Equal(t, "http", file.Import("", "net/http"))
		assert.Equal(t, genDecl(file), `import "net/http"`)
	})
	t.Run("initial with empty ident", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		_ = file.Import("", "net/http")
		_ = file.Import("", "html/template")
		assert.Equal(t, genDecl(file), `import (
	"html/template"
	"net/http"
)`)
	})
	t.Run("it respects order", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		_ = file.Import("", "html/template")
		_ = file.Import("", "net/http")
		assert.Equal(t, genDecl(file), `import (
	"html/template"
	"net/http"
)`)
	})
	t.Run("it returns the registered identifier", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		_ = file.Import("t", "html/template")
		assert.Equal(t, "t", file.Import("", "html/template"))
	})
	t.Run("it returns the package path base", func(t *testing.T) {
		pl, err := loadPkg()
		require.NoError(t, err)
		fSet := fileSet()

		wd, err := workingDir()
		require.NoError(t, err)

		file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
		require.NoError(t, err)
		_ = file.Import("", "html/template")
		assert.Equal(t, "template", file.Import("", "html/template"))
	})
}
