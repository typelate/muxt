package check_test

import (
	"cmp"
	"embed"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	htmltemplate "html/template"
	"io"
	"strings"
	"sync"
	"testing"
	texttemplate "text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

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

// TestTemplatesTemplateBridgeKeepsTheSetName states that the bridged set,
// as html/template's would, still holds a template declared with New that
// nothing was parsed under.
func TestTemplatesTemplateBridgeKeepsTheSetName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, dir, "main.go", `package main

import ("html/template"; "os")

var ts = template.Must(template.New("scratch").New("page").Parse(`+"`"+`{{define "a"}}x{{end}}`+"`"+`))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "a", nil) }
`)
	pkg := loadTestPackage(t, dir)
	ts, err := check.LoadTemplates(pkg, "ts")
	require.NoError(t, err)

	tmpl, ok := ts.Template()
	require.True(t, ok)
	scratch := tmpl.Lookup("scratch")
	require.NotNil(t, scratch, "the never parsed set name is still in the set")
	assert.Nil(t, scratch.Tree, "the never parsed set name has no tree")
}

//go:embed testdata/load/*.gohtml
var loadFS embed.FS

var loadHTMLTemplates = htmltemplate.Must(htmltemplate.New("load").Funcs(htmltemplate.FuncMap{
	"upper": strings.ToUpper,
}).ParseFS(loadFS, "testdata/load/*.gohtml"))

//lint:ignore U1000 it is loaded from source by check.LoadTemplates
var loadEscaperTemplates = htmltemplate.Must(htmltemplate.New("escapers").Parse(`{{define "link"}}<a href="/search?q={{urlquery .Query}}" onclick="{{js .Query}}">{{html .Query}}</a>{{end}}`))

var loadTextTemplates = texttemplate.Must(texttemplate.New("plain").Parse(`{{define "note"}}{{.Body}}{{end}}`))

var loadTestPkg = sync.OnceValue(func() []*packages.Package {
	packageList, loadErr := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedDeps | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedEmbedFiles,
		Tests: true,
	}, ".")
	if loadErr != nil {
		panic(loadErr)
	}
	return packageList
})

func TestLoadTemplates_templateVariables(t *testing.T) {
	testPkg := find(t, loadTestPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	t.Run("html template variable parsed from embedded files", func(t *testing.T) {
		lt, err := check.LoadTemplates(testPkg, "loadHTMLTemplates")
		require.NoError(t, err)

		ts, ok := lt.HTML()
		require.True(t, ok, "the variable holds an html/template value")
		assert.NotNil(t, ts.Lookup("greeting"))
		require.NotNil(t, loadHTMLTemplates.Lookup("greeting"),
			"the loaded set agrees with the runtime value the variable holds")
		_, ok = lt.Text()
		assert.False(t, ok)

		tree, ok := lt.FindTree("greeting")
		require.True(t, ok)
		assert.Equal(t, "greeting", tree.Name)

		def, ok := lt.FindDefinition("greeting")
		require.True(t, ok)
		require.True(t, def.Define.IsValid(), "definitions from parsed files carry define spans")
		assert.True(t, strings.HasSuffix(def.Define.Filename, "testdata/load/index.gohtml"), "got %q", def.Define.Filename)
		assert.Equal(t, 1, def.Define.Line)
		assert.Equal(t, 1, def.Define.Column)

		def, ok = lt.FindDefinition("farewell")
		require.True(t, ok)
		assert.Equal(t, 3, def.Define.Line)

		sig, ok := lt.Functions()["upper"]
		require.True(t, ok, "functions collected from the Funcs call")
		assert.Equal(t, 1, sig.Params().Len())
	})

	t.Run("text template variable parsed from a literal", func(t *testing.T) {
		lt, err := check.LoadTemplates(testPkg, "loadTextTemplates")
		require.NoError(t, err)

		ts, ok := lt.Text()
		require.True(t, ok, "the variable holds a text/template value")
		assert.NotNil(t, ts.Lookup("note"))
		require.NotNil(t, loadTextTemplates.Lookup("note"),
			"the loaded set agrees with the runtime value the variable holds")
		_, ok = lt.HTML()
		assert.False(t, ok)

		def, ok := lt.FindDefinition("note")
		require.True(t, ok)
		require.True(t, def.Define.IsValid(), "definitions in Go string literals span the Go source")
		assert.True(t, strings.HasSuffix(def.Define.Filename, "load_test.go"), "got %q", def.Define.Filename)
	})

	t.Run("unknown variable", func(t *testing.T) {
		_, err := check.LoadTemplates(testPkg, "noSuchVariable")
		require.ErrorContains(t, err, "noSuchVariable not found")
	})
}

func TestParseNodePosition_loadedTemplate(t *testing.T) {
	testPkg := find(t, loadTestPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	lt, err := check.LoadTemplates(testPkg, "loadHTMLTemplates")
	require.NoError(t, err)

	tree, ok := lt.FindTree("farewell")
	require.True(t, ok)

	pos := check.ParseNodePosition(tree, tree.Root)
	assert.Equal(t, cmp.Or(tree.Filepath, tree.ParseName), pos.Filename)
	assert.Equal(t, 3, pos.Line, "farewell is defined on the third line")
	assert.Equal(t, 22, pos.Column, "the body starts after the define clause, one based")
}

func TestCollectedFunctions(t *testing.T) {
	testPkg := find(t, loadTestPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	lt, err := check.LoadTemplates(testPkg, "loadHTMLTemplates")
	require.NoError(t, err)

	collected := lt.CollectedFunctions()
	assert.Len(t, collected, 1, "only the Funcs-registered function, no defaults")
	_, ok := collected["upper"]
	assert.True(t, ok)

	_, ok = lt.Functions()["printf"]
	assert.True(t, ok, "the full set keeps the defaults")
}

// TestLoadTemplates_escaperFunctions checks that the escaper built-ins
// (urlquery, js, html) are in the loaded function set, so a Global built
// from it does not report them as unknown functions.
func TestLoadTemplates_escaperFunctions(t *testing.T) {
	testPkg := find(t, loadTestPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	lt, err := check.LoadTemplates(testPkg, "loadEscaperTemplates")
	require.NoError(t, err)

	tree, ok := lt.FindTree("link")
	require.True(t, ok)
	global := check.NewGlobal(testPkg.Types, testPkg.Fset, lt, lt.Functions())
	data := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Query", types.Typ[types.String], false),
	}, nil)

	require.NoError(t, check.Execute(global, tree, data))
}

// renderGreeting is discovered by ExecuteTemplateCalls through the
// loadHTMLTemplates variable object.
func renderGreeting(w io.Writer) error {
	return loadHTMLTemplates.ExecuteTemplate(w, "greeting", struct{ Name string }{Name: "test"})
}

// renderShadowed must not be discovered: the identifier shares the
// variable's name but resolves to a different object.
func renderShadowed(w io.Writer) error {
	loadHTMLTemplates := htmltemplate.Must(htmltemplate.New("shadow").Parse(`{{define "greeting"}}shadow{{end}}`))
	return loadHTMLTemplates.ExecuteTemplate(w, "greeting", 1)
}

func TestExecuteTemplateCalls(t *testing.T) {
	testPkg := find(t, loadTestPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	lt, err := check.LoadTemplates(testPkg, "loadHTMLTemplates")
	require.NoError(t, err)

	require.NoError(t, renderGreeting(io.Discard))
	require.NoError(t, renderShadowed(io.Discard))

	var calls []check.Entry
	for call := range lt.ExecuteTemplateCalls() {
		calls = append(calls, call)
	}
	require.Len(t, calls, 1, "the shadowed identifier resolves to a different object and is not reported")

	call := calls[0]
	assert.Equal(t, "greeting", call.TemplateName)
	assert.Equal(t, "struct{Name string}", types.TypeString(call.DataType, nil))
	// Upstream's ExecuteTemplateCall carries the Definition; the fork's
	// Entry carries the tree, and the definition is one lookup away.
	require.NotNil(t, call.Tree, "the named template's tree rides along")
	def, ok := lt.FindDefinition(call.TemplateName)
	require.True(t, ok)
	require.True(t, def.Define.IsValid())
	assert.Equal(t, "greeting", def.Name)
}

func TestLoadTemplatesWithoutTypeInfo(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "templates.go", `package x

import "html/template"

var templates = template.New("x")
`, parser.SkipObjectResolution)
	require.NoError(t, err)

	_, err = check.LoadTemplates(&packages.Package{
		PkgPath: "example.com/x",
		Fset:    fileSet,
		Syntax:  []*ast.File{file},
	}, "templates")
	require.ErrorContains(t, err, "loaded without type information",
		"a silently empty result would be undiagnosable")
}
