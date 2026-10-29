package check_test

import (
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

// analyzeModule writes files into a temp module, loads it, and returns every
// warning AnalyzePackage produces across its packages. Used for the
// package-level warnings (W001, W002) that are located in Go source rather
// than a single template.
func analyzeModule(t *testing.T, files map[string]string) []check.Warning {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns |
			packages.NeedEmbedFiles | packages.NeedImports | packages.NeedModule |
			packages.NeedDeps,
		Dir: dir,
	}, "./...")
	require.NoError(t, err)
	var all []check.Warning
	for _, pkg := range pkgs {
		ws, _ := check.AnalyzePackage(pkg, nil)
		all = append(all, ws...)
	}
	return all
}

func hasWarning(ws []check.Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}

// warningsOf type-checks src against data and returns the warnings the
// checker emitted (asserting the check itself did not error).
func warningsOf(t *testing.T, src string, data types.Type) []check.Warning {
	t.Helper()
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	tp, err := template.New[*types.Signature]("main").Parse(src)
	require.NoError(t, err)
	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tp, check.DefaultFunctions(testPkg.Types))
	var ws []check.Warning
	global.InspectWarning = func(w check.Warning) { ws = append(ws, w) }
	tree, ok := tp.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, data))
	return ws
}

func TestWarnDeadBranch(t *testing.T) {
	data := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "B", types.Typ[types.Bool], false),
	}, nil)

	// {{if false}} body is dead.
	ws := warningsOf(t, `{{if false}}dead{{end}}`, data)
	require.Len(t, ws, 1)
	assert.Equal(t, check.WarnDeadBranch, ws[0].Code)
	assert.Contains(t, ws[0].Message, "always false")

	// {{if true}}...{{else}} the else is dead.
	ws = warningsOf(t, `{{if true}}live{{else}}dead{{end}}`, data)
	require.Len(t, ws, 1)
	assert.Equal(t, check.WarnDeadBranch, ws[0].Code)
	assert.Contains(t, ws[0].Message, "always true")

	// No dead branch: a true condition with no else, and a real condition.
	assert.Empty(t, warningsOf(t, `{{if true}}live{{end}}`, data))
	assert.Empty(t, warningsOf(t, `{{if .B}}maybe{{end}}`, data))
	// An empty dead body is not worth flagging.
	assert.Empty(t, warningsOf(t, `{{if false}}{{end}}`, data))
}

func TestWarnInterfaceFieldAccess(t *testing.T) {
	anyType := types.NewInterfaceType(nil, nil).Complete()

	// A field access on an interface dot warns (and does not error).
	ws := warningsOf(t, `{{.Title}}`, anyType)
	require.Len(t, ws, 1)
	assert.Equal(t, check.WarnInterfaceFieldAccess, ws[0].Code)
	assert.Contains(t, ws[0].Message, "Title")

	// A chain on an interface warns once, not once per segment.
	assert.Len(t, warningsOf(t, `{{.A.B.C}}`, anyType), 1)

	// A concrete struct is verified, not warned.
	data := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Title", types.Typ[types.String], false),
	}, nil)
	assert.Empty(t, warningsOf(t, `{{.Title}}`, data))
}

func TestWarnUnusedVariable(t *testing.T) {
	data := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Title", types.Typ[types.String], false),
	}, nil)

	// Declared, never referenced.
	ws := warningsOf(t, `{{$x := .Title}}<h1>{{.Title}}</h1>`, data)
	require.Len(t, ws, 1)
	assert.Equal(t, check.WarnUnusedVariable, ws[0].Code)
	assert.Contains(t, ws[0].Message, "$x")

	// Declared and used (directly, and via a field chain).
	assert.Empty(t, warningsOf(t, `{{$x := .Title}}{{$x}}`, data))
	assert.Empty(t, warningsOf(t, `{{$x := .}}{{$x.Title}}`, data))

	// Loop variables of {{range}} are exempt even when partially unused.
	assert.Empty(t, warningsOf(t, `{{range $i, $e := .Names}}{{$e}}{{end}}`,
		types.NewStruct([]*types.Var{
			types.NewField(0, nil, "Names", types.NewSlice(types.Typ[types.String]), false),
		}, nil)))
}

func TestWarnNonStaticName(t *testing.T) {
	// A variable template name cannot be checked statically.
	ws := analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"html/template"
	"os"
)

var ts = template.Must(template.New("app").Parse("hi"))

func render(name string) { _ = ts.ExecuteTemplate(os.Stdout, name, nil) }

func main() {}
`,
	})
	assert.True(t, hasWarning(ws, check.WarnNonStaticName), "non-literal template name warns W001")

	// A string-literal name does not warn W001.
	ws = analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"html/template"
	"os"
)

var ts = template.Must(template.New("app").Parse("hi"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "app", nil) }
`,
	})
	assert.False(t, hasWarning(ws, check.WarnNonStaticName))
}

func TestWarnUnusedTemplate(t *testing.T) {
	ws := analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"embed"
	"html/template"
	"os"
)

//go:embed *.gohtml
var source embed.FS

var ts = template.Must(template.New("app").ParseFS(source, "*.gohtml"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "index.gohtml", nil) }
`,
		"index.gohtml":  `<h1>hi</h1>`,
		"unused.gohtml": `<p>never referenced</p>`,
	})
	assert.True(t, hasWarning(ws, check.WarnUnusedTemplate), "unused.gohtml warns W002")
	// The message names the unused template, not the used one.
	var msg string
	for _, w := range ws {
		if w.Code == check.WarnUnusedTemplate {
			msg = w.Message
		}
	}
	assert.Contains(t, msg, "unused.gohtml")
	assert.NotContains(t, msg, "index.gohtml")
}

// TestWarnUnusedTemplateSkipsDefineOnlyFile guards that a file consisting of
// only {{define}} blocks (the muxt/htt-generate route-file shape) is NOT
// reported as an unused file template: htt keeps defines inline and htt.Walk
// descends into their bodies, so hasRenderableContent must treat a define as
// opaque. The defines themselves are used, via routes.gohtml's {{template}}.
func TestWarnUnusedTemplateSkipsDefineOnlyFile(t *testing.T) {
	ws := analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"embed"
	"html/template"
	"os"
)

//go:embed *.gohtml
var source embed.FS

var ts = template.Must(template.New("app").ParseFS(source, "*.gohtml"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "page.gohtml", nil) }
`,
		// page.gohtml renders the partial, so the partial is used; the
		// partials file itself is only defines + whitespace and must not
		// be flagged unused.
		"page.gohtml":     `{{template "greeting" .}}`,
		"partials.gohtml": "{{define \"greeting\"}}<p>hi</p>{{end}}\n",
	})
	for _, w := range ws {
		if w.Code == check.WarnUnusedTemplate {
			assert.NotContains(t, w.Message, "partials.gohtml",
				"a define-only file must not be reported as an unused template")
		}
	}
}

func TestWarnInconsistentTemplateTypes(t *testing.T) {
	// "header" is invoked with a struct at one site and an int at another.
	ws := analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"embed"
	"html/template"
	"os"
)

//go:embed *.gohtml
var source embed.FS

type PageData struct{ Title string }
type Data struct {
	Page  PageData
	Count int
}

var ts = template.Must(template.New("app").ParseFS(source, "*.gohtml"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "page", Data{}) }
`,
		"page.gohtml": `{{define "page"}}{{template "header" .Page}}{{template "header" .Count}}{{end}}` +
			`{{define "header"}}<h1>{{.}}</h1>{{end}}`,
	})
	assert.True(t, hasWarning(ws, check.WarnInconsistentTemplate), "header invoked with struct and int warns W007")

	// Consistent invocations (same type both sites) do not warn.
	ws = analyzeModule(t, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.25.0\n",
		"main.go": `package main

import (
	"embed"
	"html/template"
	"os"
)

//go:embed *.gohtml
var source embed.FS

type PageData struct{ Title string }
type Data struct {
	A PageData
	B PageData
}

var ts = template.Must(template.New("app").ParseFS(source, "*.gohtml"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "page", Data{}) }
`,
		"page.gohtml": `{{define "page"}}{{template "header" .A}}{{template "header" .B}}{{end}}` +
			`{{define "header"}}<h1>{{.Title}}</h1>{{end}}`,
	})
	assert.False(t, hasWarning(ws, check.WarnInconsistentTemplate))
}

func TestWarnUnguardedPointer(t *testing.T) {
	inner := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Name", types.Typ[types.String], false),
	}, nil)
	ptrInner := types.NewPointer(inner)
	page := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Title", types.Typ[types.String], false),
	}, nil)
	ptrPage := types.NewPointer(page)
	w := func(src string, data types.Type) bool {
		return hasWarning(warningsOf(t, src, data), check.WarnUnguardedPointer)
	}

	// Pointer dot, unguarded field access.
	assert.True(t, w(`{{.Title}}`, ptrPage))
	// Guarded by {{if .}} and by {{with .}}.
	assert.False(t, w(`{{if .}}{{.Title}}{{end}}`, ptrPage))
	assert.False(t, w(`{{with .}}{{.Title}}{{end}}`, ptrPage))
	// A value dot never warns.
	assert.False(t, w(`{{.Title}}`, page))

	// Pointer field: {{.S.Name}} dereferences .S.
	data := types.NewStruct([]*types.Var{types.NewField(0, nil, "S", ptrInner, false)}, nil)
	assert.True(t, w(`{{.S.Name}}`, data))
	assert.False(t, w(`{{if .S}}{{.S.Name}}{{end}}`, data))
	// {{if and .S ...}} guards .S in the body via short-circuit.
	assert.False(t, w(`{{if and .S true}}{{.S.Name}}{{end}}`, data))

	// A templatecheck:"nonil" tag suppresses the warning.
	tagged := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "S", ptrInner, false),
	}, []string{`templatecheck:"nonil"`})
	assert.False(t, w(`{{.S.Name}}`, tagged))
}

func TestWarnUnguardedPointerAndShortCircuit(t *testing.T) {
	inner := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Name", types.Typ[types.String], false),
	}, nil)
	data := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "S", types.NewPointer(inner), false),
	}, nil)
	w := func(src string) bool { return hasWarning(warningsOf(t, src, data), check.WarnUnguardedPointer) }

	// .S.Name inside the and condition is guarded by the preceding .S operand.
	assert.False(t, w(`{{if and .S (eq .S.Name "x")}}ok{{end}}`))
	// And the body access is guarded too.
	assert.False(t, w(`{{if and .S true}}{{.S.Name}}{{end}}`))
	// Without the guard, it still warns.
	assert.True(t, w(`{{eq .S.Name "x"}}`))
}

func TestWarnUnguardedPointerNoFalsePositives(t *testing.T) {
	user := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Name", types.Typ[types.String], false),
	}, nil)
	ptrUser := types.NewPointer(user)
	w := func(src string, data types.Type) bool {
		return hasWarning(warningsOf(t, src, data), check.WarnUnguardedPointer)
	}

	// {{range}} over a slice of pointers, accessing a field, is idiomatic and
	// must not warn — via dot and via a loop variable.
	sliceData := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "Users", types.NewSlice(ptrUser), false),
	}, nil)
	assert.False(t, w(`{{range .Users}}{{.Name}}{{end}}`, sliceData))
	assert.False(t, w(`{{range $u := .Users}}{{$u.Name}}{{end}}`, sliceData))

	// {{range}} over a map of pointers likewise.
	mapData := types.NewStruct([]*types.Var{
		types.NewField(0, nil, "M", types.NewMap(types.Typ[types.String], ptrUser), false),
	}, nil)
	assert.False(t, w(`{{range .M}}{{.Name}}{{end}}`, mapData))
}

func TestWarnUnguardedPointerMethodVsField(t *testing.T) {
	mod := func(page string) []check.Warning {
		return analyzeModule(t, map[string]string{
			"go.mod": "module example.com/app\n\ngo 1.25.0\n",
			"main.go": `package main

import (
	"embed"
	"html/template"
	"os"
)

//go:embed page.gohtml
var source embed.FS

type Inner struct{ Field string }

func (i *Inner) Name() string {
	if i == nil {
		return ""
	}
	return i.Field
}

type Data struct{ S *Inner }

var ts = template.Must(template.New("app").ParseFS(source, "page.gohtml"))

func main() { _ = ts.ExecuteTemplate(os.Stdout, "page.gohtml", Data{}) }
`,
			"page.gohtml": page,
		})
	}
	// A method call through a pointer may be nil-safe: not flagged.
	assert.False(t, hasWarning(mod(`{{.S.Name}}`), check.WarnUnguardedPointer))
	// A field access through the same pointer is flagged.
	assert.True(t, hasWarning(mod(`{{.S.Field}}`), check.WarnUnguardedPointer))
}
