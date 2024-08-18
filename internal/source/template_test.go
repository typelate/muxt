package source_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/crhntr/muxt/internal/source"
)

func TestTemplates(t *testing.T) {
	t.Run("simple.txtar", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/simple.txtar"))
		goFiles, fileSet := parseGo(t, dir)
		ts, err := source.Templates(dir, "templates", fileSet, goFiles, []string{
			filepath.Join(dir, "index.gohtml"),
			filepath.Join(dir, "form.gohtml"),
		})
		assert.NoError(t, err)
		var names []string
		for _, t := range ts.Templates() {
			names = append(names, t.Name())
		}
		slices.Sort(names)
		assert.Equal(t, []string{"create", "form.gohtml", "home", "index.gohtml", "update"}, names)
	})
}

func createTestDir(t *testing.T, filename string) string {
	t.Helper()
	dir := t.TempDir()
	archive, err := txtar.ParseFile(filepath.FromSlash(filename))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.Files {
		output := filepath.Join(dir, filepath.FromSlash(file.Name))
		if err := os.WriteFile(output, file.Data, 0666); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func parseGo(t *testing.T, dir string) ([]*ast.File, *token.FileSet) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	set := token.NewFileSet()
	var files []*ast.File
	for _, match := range matches {
		file, err := parser.ParseFile(set, match, nil, parser.ParseComments|parser.AllErrors|parser.SkipObjectResolution)
		require.NoError(t, err)
		files = append(files, file)
	}
	return files, set
}
