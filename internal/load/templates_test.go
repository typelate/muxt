package load_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
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
	_, pl, err := load.Packages(dir)
	require.NoError(t, err)
	pkg, ok := load.PackageInDirectory(pl, dir)
	require.True(t, ok)

	t.Run("parses the embedded files", func(t *testing.T) {
		lt, ts, err := load.HTMLTemplates("templates", pkg)
		require.NoError(t, err)
		functions := lt.CollectedFunctions()

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
		_, _, err := load.HTMLTemplates("nope", pkg)
		require.ErrorContains(t, err, "variable nope not found")
	})

	t.Run("a variable locates its definitions and functions", func(t *testing.T) {
		variable, err := load.Variable(pkg, "templates")
		require.NoError(t, err)
		require.NotNil(t, variable.Set)

		def, ok := variable.Definitions["home"]
		require.True(t, ok, "definitions resolve for file-parsed templates")
		require.True(t, def.Define.IsValid())
		assert.Equal(t, "index.gohtml", filepath.Base(def.Define.Filename))
		assert.Contains(t, variable.Funcs, "upper", "Funcs holds the Funcs-registered functions")
		assert.Len(t, variable.Funcs, 1, "Funcs holds only what Funcs registered")
		assert.Contains(t, variable.Functions, "upper")
		assert.Contains(t, variable.Functions, "printf", "Functions holds the builtins a template may call too")
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
	_, pl, err := load.Packages(dir)
	require.NoError(t, err)
	pkg, ok := load.PackageInDirectory(pl, dir)
	require.True(t, ok)

	// Muxt introspects trees without executing, so a text/template
	// variable loads through an html/template value with the same trees.
	_, ts, err := load.HTMLTemplates("texts", pkg)
	require.NoError(t, err)
	require.NotNil(t, ts.Lookup("note"))
}

// TestPackageInADirectoryNamedLikeAGoFile states that the package a command
// runs on is found by its directory, even when the directory's name ends in
// .go.
func TestPackageInADirectoryNamedLikeAGoFile(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "app.go")
	require.NoError(t, os.Mkdir(dir, 0o755))
	for name, content := range map[string]string{
		"go.mod":       "module scratch\n\ngo 1.24\n",
		"main.go":      templatesGo,
		"index.gohtml": `{{define "home"}}Hello{{end}}`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	_, pl, err := load.Packages(dir)
	require.NoError(t, err)

	pkg, err := load.Package(dir, pl, []string{"templates"})
	require.NoError(t, err)
	assert.Equal(t, "scratch", pkg.Types.Path())

	// Every command reads the package through Package now, the mutation
	// run included.
	require.Len(t, pkg.Variables, 1)
	require.NotNil(t, pkg.Variables[0].Set.Lookup("home"))
}
