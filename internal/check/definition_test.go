package check_test

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

// formatDefinition renders a definition the way upstream's
// check-templates -definitions listing does: define, name, end, and the
// quoted name literal, with paths relative to dir.
func formatDefinition(t *testing.T, dir string, def check.Definition) string {
	t.Helper()
	rel := func(s check.Span) string {
		if !s.IsValid() {
			return s.String()
		}
		name, err := filepath.Rel(dir, s.Filename)
		require.NoError(t, err)
		s.Filename = name
		return s.String()
	}
	return rel(def.Define) + "\t" + `"` + def.Name + `"` + "\t" + rel(def.End) + "\t" + rel(def.TemplateName)
}

// The expected listings below are upstream's cmd/check-templates
// pass_definitions*.txt fixtures at v0.8.3, with $WORK/ dropped.
func TestTemplatesFindDefinition(t *testing.T) {
	t.Run("definitions in an embedded template file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", `package main

import (
	"embed"
	"html/template"
	"net/http"
)

var (
	//go:embed *.gohtml
	source embed.FS

	templates = template.Must(template.ParseFS(source, "*"))
)

type Page struct {
	Title string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	_ = templates.ExecuteTemplate(w, "greeting", Page{Title: "Home"})
}
`)
		writeFile(t, dir, "index.gohtml", "{{define \"greeting\"}}Hi {{template \"name\" .}}{{end}}\n{{define \"name\"}}{{.Title}}{{end}}\n")
		pkg := loadTestPackage(t, dir)
		dir = filepath.Dir(pkg.GoFiles[0])

		ts, err := check.LoadTemplates(pkg, "templates")
		require.NoError(t, err)

		for name, want := range map[string]string{
			"greeting": "index.gohtml:1:1+21\t\"greeting\"\tindex.gohtml:1:46+7\tindex.gohtml:1:10+10",
			"name":     "index.gohtml:2:1+17\t\"name\"\tindex.gohtml:2:28+7\tindex.gohtml:2:10+6",
		} {
			def, ok := ts.FindDefinition(name)
			require.True(t, ok, "FindDefinition(%q)", name)
			assert.Equal(t, want, formatDefinition(t, dir, def), "FindDefinition(%q)", name)
			tree, _ := ts.FindTree(name)
			assert.Same(t, tree, def.Tree, "FindDefinition(%q).Tree should be the tree the checker walks", name)
		}
		_, ok := ts.FindDefinition("missing")
		assert.False(t, ok, "FindDefinition(%q)", "missing")
	})

	t.Run("a definition in a Go string literal", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", "package main\n\nimport (\n\t\"fmt\"\n\t\"html/template\"\n\t\"net/http\"\n)\n\n"+
			"var templates = template.Must(template.New(\"app\").Parse(`{{define \"greeting\"}}Hi{{end}}`))\n"+`
type Page struct {
	Title string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	_ = templates.ExecuteTemplate(w, "greeting", Page{Title: "Home"})
}

var _ = fmt.Sprint
`)
		pkg := loadTestPackage(t, dir)
		dir = filepath.Dir(pkg.GoFiles[0])

		ts, err := check.LoadTemplates(pkg, "templates")
		require.NoError(t, err)

		def, ok := ts.FindDefinition("greeting")
		require.True(t, ok)
		assert.Equal(t, "main.go:9:58+21\t\"greeting\"\tmain.go:9:81+7\tmain.go:9:67+10", formatDefinition(t, dir, def))
	})

	t.Run("a definition written with custom delimiters", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.25.0\n")
		writeFile(t, dir, "main.go", "package main\n\nimport \"html/template\"\n\n"+
			"var templates = template.Must(template.New(\"app\").Delims(\"[[\", \"]]\").Parse(`[[define \"greeting\"]]Hi[[end]]`))\n")
		pkg := loadTestPackage(t, dir)
		dir = filepath.Dir(pkg.GoFiles[0])

		ts, err := check.LoadTemplates(pkg, "templates")
		require.NoError(t, err)

		def, ok := ts.FindDefinition("greeting")
		require.True(t, ok)
		assert.Equal(t, "main.go:5:77+21\t\"greeting\"\tmain.go:5:100+7\tmain.go:5:86+10", formatDefinition(t, dir, def))
	})
}

func TestPackageInspectorsReceiveDefinitions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

import (
	"embed"
	"html/template"
	"net/http"
)

var (
	//go:embed *.gohtml
	source embed.FS

	templates = template.Must(template.ParseFS(source, "*"))
)

type Page struct {
	Title string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	_ = templates.ExecuteTemplate(w, "greeting", Page{Title: "Home"})
}
`)
	writeFile(t, dir, "index.gohtml", "{{define \"greeting\"}}Hi {{template \"name\" .}}{{end}}\n{{define \"name\"}}{{.Title}}{{end}}\n")
	pkg := loadTestPackage(t, dir)
	dir = filepath.Dir(pkg.GoFiles[0])

	var got []string
	err := check.Package(pkg,
		func(_ *ast.CallExpr, _ *template.Tree, _ types.Type, def check.Definition) {
			got = append(got, formatDefinition(t, dir, def))
		},
		func(_ *template.TemplateNode, _ *template.Tree, _ types.Type, def check.Definition) {
			got = append(got, formatDefinition(t, dir, def))
		},
	)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"index.gohtml:1:1+21\t\"greeting\"\tindex.gohtml:1:46+7\tindex.gohtml:1:10+10",
		"index.gohtml:2:1+17\t\"name\"\tindex.gohtml:2:28+7\tindex.gohtml:2:10+6",
	}, got)
}

// packageCallDefinitions runs Package over a module written to a temp
// directory and lists the definition each ExecuteTemplate call receives.
func packageCallDefinitions(t *testing.T, files map[string]string) ([]string, error) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.25.0\n")
	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	pkg := loadTestPackage(t, dir)
	dir = filepath.Dir(pkg.GoFiles[0])

	var got []string
	err := check.Package(pkg, func(_ *ast.CallExpr, _ *template.Tree, _ types.Type, def check.Definition) {
		got = append(got, formatDefinition(t, dir, def))
	}, nil)
	return got, err
}

// The cases below are upstream's cmd/check-templates fixtures at v0.8.3
// (pass_definitions_delims, pass_definitions_additional_parsefs,
// pass_definitions_same_name, err_delims_additional_parsefs), run through
// Package.
func TestPackageDefinitionsAcrossStatements(t *testing.T) {
	t.Run("delimiters set in one statement and a literal parsed in another", func(t *testing.T) {
		got, err := packageCallDefinitions(t, map[string]string{"main.go": `package main

import (
	"fmt"
	"html/template"
	"net/http"
)

var templates = template.New("app").Delims("[[", "]]")

var _ = template.Must(templates.Parse(` + "`" + `[[define "greeting"]]Hi[[end]]` + "`" + `))

type Page struct {
	Title string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	_ = templates.ExecuteTemplate(w, "greeting", Page{Title: "Home"})
}

var _ = fmt.Sprint
`})
		require.NoError(t, err)
		assert.Equal(t, []string{
			"main.go:11:40+21\t\"greeting\"\tmain.go:11:63+7\tmain.go:11:49+10",
		}, got)
	})

	t.Run("a definition parsed by a later ParseFS", func(t *testing.T) {
		got, err := packageCallDefinitions(t, map[string]string{
			"main.go": `package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed *.gohtml
var source embed.FS

type Page struct {
	Title string
}

var ts = template.Must(template.New("app").ParseFS(source, "index.gohtml"))

func setup() {
	template.Must(ts.ParseFS(source, "about.gohtml"))
}

func render() {
	_ = ts.ExecuteTemplate(io.Discard, "farewell", Page{Title: "Home"})
}

var _ = fmt.Sprint
`,
			"index.gohtml": "<h1>{{.Title}}</h1>\n",
			"about.gohtml": "{{define \"farewell\"}}Bye{{end}}\n",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{
			"about.gohtml:1:1+21\t\"farewell\"\tabout.gohtml:1:25+7\tabout.gohtml:1:10+10",
		}, got)
	})

	t.Run("two template variables define the same name", func(t *testing.T) {
		got, err := packageCallDefinitions(t, map[string]string{"main.go": `package main

import (
	"fmt"
	"html/template"
	"net/http"
)

var admin = template.Must(template.New("admin").Parse(` + "`" + `{{define "row"}}ADMIN{{end}}` + "`" + `))
var public = template.Must(template.New("public").Parse(` + "`" + `{{define "row"}}PUBLIC{{end}}` + "`" + `))

type Page struct {
	Title string
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	_ = admin.ExecuteTemplate(w, "row", Page{Title: "Home"})
}

func handlePublic(w http.ResponseWriter, r *http.Request) {
	_ = public.ExecuteTemplate(w, "row", Page{Title: "Home"})
}

var _ = fmt.Sprint
`})
		require.NoError(t, err)
		assert.Equal(t, []string{
			"main.go:9:56+16\t\"row\"\tmain.go:9:77+7\tmain.go:9:65+5",
			"main.go:10:58+16\t\"row\"\tmain.go:10:80+7\tmain.go:10:67+5",
		}, got)
	})

	t.Run("delimiters set on the declaration reach a later ParseFS", func(t *testing.T) {
		_, err := packageCallDefinitions(t, map[string]string{
			"main.go": `package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.New("app").Delims("[[", "]]")

func setup() {
	template.Must(templates.ParseFS(source, "*"))
}

type Page struct {
	Title string
}

func render() {
	_ = templates.ExecuteTemplate(io.Discard, "index.gohtml", Page{Title: "Home"})
}

var _ = fmt.Sprint
`,
			"index.gohtml": "[[.Missing]]",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Missing not found on example.com/app.Page")
	})
}
