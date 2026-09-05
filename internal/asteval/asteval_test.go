package asteval_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
)

// writeModule lays out a scratch module so templates load the way muxt
// loads them in a user project: through packages.Load with type
// information and embedded files.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GOWORK", "off") // the scratch module is never part of a workspace
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	files["go.mod"] = "module scratch\n\ngo 1.24\n"
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	return dir
}

const templatesGo = `package main

import (
	"embed"
	"html/template"
	"strings"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.New("scratch").Funcs(template.FuncMap{
	"upper": strings.ToUpper,
}).ParseFS(templatesFS, "*.gohtml"))

func main() {}
`

func TestTemplates(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.go":      templatesGo,
		"index.gohtml": `{{define "home"}}Hello, {{upper .Name}}{{end}}`,
		"form.gohtml":  `{{define "create"}}<form></form>{{end}}`,
	})
	_, pl, err := asteval.LoadPackages(dir)
	require.NoError(t, err)
	pkg, ok := asteval.PackageAtFilepath(pl, dir)
	require.True(t, ok)

	t.Run("parses the embedded files", func(t *testing.T) {
		ts, functions, err := asteval.Templates("templates", pkg)
		require.NoError(t, err)

		var names []string
		for _, tmpl := range ts.Templates() {
			names = append(names, tmpl.Name())
		}
		slices.Sort(names)
		assert.Equal(t, []string{"create", "form.gohtml", "home", "index.gohtml", "scratch"}, names)

		_, ok := functions["upper"]
		assert.True(t, ok, "functions holds only the Funcs-registered functions")
		assert.Len(t, functions, 1)
	})

	t.Run("unknown variable", func(t *testing.T) {
		_, _, err := asteval.Templates("nope", pkg)
		require.ErrorContains(t, err, "variable nope not found")
	})

	t.Run("load templates wires the global", func(t *testing.T) {
		_, global, ts, err := asteval.LoadTemplates(dir, "templates", pl)
		require.NoError(t, err)
		require.NotNil(t, ts)

		def, ok := global.Definitions.FindDefinition("home")
		require.True(t, ok, "definitions resolve for file-parsed templates")
		require.True(t, def.Define.IsValid())
		assert.Equal(t, "index.gohtml", filepath.Base(def.Define.Filename))
	})
}

func TestTemplatesText(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.go": `package main

import "text/template"

var texts = template.Must(template.New("t").Parse(` + "`{{define \"note\"}}hi{{end}}`" + `))

func main() {}
`,
	})
	_, pl, err := asteval.LoadPackages(dir)
	require.NoError(t, err)
	pkg, ok := asteval.PackageAtFilepath(pl, dir)
	require.True(t, ok)

	// Muxt introspects trees without executing, so a text/template
	// variable loads through an html/template value with the same trees.
	ts, _, err := asteval.Templates("texts", pkg)
	require.NoError(t, err)
	require.NotNil(t, ts.Lookup("note"))
}
