package load_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/load/loadtest"
)

const hydrateServer = `package server

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))

type Server struct{}

func Render(w io.Writer) error {
	return templates.ExecuteTemplate(w, "page", nil)
}
`

// TestHydration states what a command's configuration reads from a load,
// and which failure a configuration naming several missing things reports.
func TestHydration(t *testing.T) {
	dir := t.TempDir()
	pl := loadtest.Package(t, dir, "example.com/server", map[string]string{
		"server.go":   hydrateServer,
		"page.gohtml": `{{define "page"}}<p>{{.}}</p>{{end}}`,
	})

	t.Run("the package, its receiver, and each variable", func(t *testing.T) {
		pkg, receiver, err := load.RoutesSource(dir, pl, analysis.DefinitionsConfiguration{ReceiverType: "Server", TemplatesVariables: []string{"templates"}})
		require.NoError(t, err)
		assert.Equal(t, "Server", receiver.Obj().Name())
		require.Len(t, pkg.Variables, 1)
		variable := pkg.Variables[0]
		assert.Equal(t, "templates", variable.Name)
		require.Len(t, variable.Calls, 1)
		assert.Equal(t, "page", variable.Calls[0].Template)
		assert.Equal(t, filepath.Join(dir, "server.go"), variable.Calls[0].Position.Filename)
		_, ok := variable.NamePosition("page")
		assert.True(t, ok, "the page's definition is located")
	})

	t.Run("no receiver named is none looked up", func(t *testing.T) {
		_, receiver, err := load.RoutesSource(dir, pl, analysis.DefinitionsConfiguration{TemplatesVariables: []string{"templates"}})
		require.NoError(t, err)
		assert.Nil(t, receiver)
	})

	t.Run("a missing receiver is reported before a missing variable", func(t *testing.T) {
		_, _, err := load.RoutesSource(dir, pl, analysis.DefinitionsConfiguration{ReceiverType: "Srever", TemplatesVariables: []string{"nope"}})
		require.EqualError(t, err, "could not find receiver type Srever in example.com/server; did you mean Server?")
	})

	t.Run("the first variable that does not evaluate", func(t *testing.T) {
		_, err := load.Package(dir, pl, []string{"templates", "nope", "neither"})
		require.EqualError(t, err, "variable nope not found in package example.com/server")
	})

	t.Run("the routes file belongs to the package in its own directory", func(t *testing.T) {
		_, _, err := load.GenerateSource(dir, pl, generate.RoutesFileConfiguration{OutputFileName: filepath.Join("sub", "routes.go"), ReceiverType: "Srever"})
		require.Error(t, err)
		assert.Equal(t, "no Go package found at "+filepath.Join(dir, "sub"), err.Error(), "a missing package is reported before a missing receiver")
	})
}
