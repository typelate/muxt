package check_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/check"
)

func TestLoadTemplates(t *testing.T) {
	t.Run("resolves a variable and its ExecuteTemplate calls", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/testpkg\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", `package main

import (
	"html/template"
	"os"
)

type Data struct {
	Name string
}

var ts = template.Must(template.New("greeting").Parse(`+"`"+`Hello, {{.Name}}!`+"`"+`))

func main() {
	ts.ExecuteTemplate(os.Stdout, "greeting", Data{Name: "world"})
}
`)
		pkg := loadTestPackage(t, dir)

		templates, err := check.LoadTemplates(pkg, "ts")
		require.NoError(t, err)
		require.NotNil(t, templates)

		require.NotNil(t, templates.Set())
		assert.NotNil(t, templates.Set().Lookup("greeting"), "resolved set should resolve the template name")
		assert.Contains(t, templates.Functions(), "html", "merged functions should include the escaper builtins")

		var calls []check.Entry
		for call := range templates.ExecuteTemplateCalls() {
			calls = append(calls, call)
		}
		require.Len(t, calls, 1)
		e := calls[0]
		assert.Equal(t, "greeting", e.TemplateName)
		assert.Contains(t, e.DataType.String(), "Data")
		assert.NotNil(t, e.Tree)
		require.NotNil(t, e.Templates)
		assert.NotNil(t, e.Templates.Lookup("greeting"))
	})

	t.Run("ExecuteTemplateCalls matches by object, not name", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/testpkg\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", `package main

import (
	"html/template"
	"os"
)

var ts = template.Must(template.New("a").Parse(`+"`"+`{{define "a"}}A{{end}}`+"`"+`))

var other = template.Must(template.New("b").Parse(`+"`"+`{{define "b"}}B{{end}}`+"`"+`))

func main() {
	ts.ExecuteTemplate(os.Stdout, "a", nil)
	other.ExecuteTemplate(os.Stdout, "b", nil)
}
`)
		pkg := loadTestPackage(t, dir)

		tsTemplates, err := check.LoadTemplates(pkg, "ts")
		require.NoError(t, err)
		var tsNames []string
		for call := range tsTemplates.ExecuteTemplateCalls() {
			tsNames = append(tsNames, call.TemplateName)
		}
		assert.Equal(t, []string{"a"}, tsNames, "only calls on ts should be reported")

		otherTemplates, err := check.LoadTemplates(pkg, "other")
		require.NoError(t, err)
		var otherNames []string
		for call := range otherTemplates.ExecuteTemplateCalls() {
			otherNames = append(otherNames, call.TemplateName)
		}
		assert.Equal(t, []string{"b"}, otherNames, "only calls on other should be reported")
	})

	t.Run("Functions includes chain-registered funcs and builtins", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/testpkg\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", `package main

import "html/template"

var ts = template.Must(template.New("greeting").Funcs(template.FuncMap{
	"greet": func() string { return "hi" },
}).Parse(`+"`"+`{{greet}}`+"`"+`))

func main() { _ = ts }
`)
		pkg := loadTestPackage(t, dir)

		templates, err := check.LoadTemplates(pkg, "ts")
		require.NoError(t, err)

		assert.Contains(t, templates.Functions(), "greet", "should include the registered func")
		assert.Contains(t, templates.Functions(), "html", "should include the escaper builtins")
	})

	t.Run("missing variable is an error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/testpkg\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", `package main

func main() {}
`)
		pkg := loadTestPackage(t, dir)

		_, err := check.LoadTemplates(pkg, "doesNotExist")
		require.ErrorContains(t, err, "not found")
	})
}

func TestTemplatesFindTree(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, dir, "main.go", `package main

import ("html/template"; "os")

var ts = template.Must(template.New("index").Parse(`+"`"+`<h1>{{.}}</h1>{{define "row"}}<td></td>{{end}}`+"`"+`))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "index", nil) }
`)
	pkg := loadTestPackage(t, dir)
	ts, err := check.LoadTemplates(pkg, "ts")
	require.NoError(t, err)

	tree, ok := ts.FindTree("row")
	require.True(t, ok)
	require.NotNil(t, tree)
	_, ok = ts.FindTree("nope")
	require.False(t, ok)
}

func TestTemplatesCollectedFunctions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, dir, "main.go", `package main

import "html/template"

var ts = template.Must(template.New("g").Funcs(template.FuncMap{
	"greet": func() string { return "hi" },
}).Parse(`+"`"+`{{greet}}`+"`"+`))

func main() { _ = ts }
`)
	pkg := loadTestPackage(t, dir)
	ts, err := check.LoadTemplates(pkg, "ts")
	require.NoError(t, err)

	collected := ts.CollectedFunctions()
	assert.Contains(t, collected, "greet")
	assert.NotContains(t, collected, "html") // an asteval default, not chain-registered
	// merged Functions() still has both:
	assert.Contains(t, ts.Functions(), "greet")
}

func TestTemplatesTemplateBridge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, dir, "main.go", `package main

import ("html/template"; "os")

var ts = template.Must(template.New("index").Parse(`+"`"+`<h1>x</h1>{{define "row"}}<td></td>{{end}}`+"`"+`))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "index", nil) }
`)
	pkg := loadTestPackage(t, dir)
	ts, err := check.LoadTemplates(pkg, "ts")
	require.NoError(t, err)

	tmpl, ok := ts.Template()
	require.True(t, ok)
	require.NotNil(t, tmpl)
	assert.NotNil(t, tmpl.Lookup("row"), "bridged template exposes defined templates")
	var names []string
	for _, sub := range tmpl.Templates() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "row")
}
