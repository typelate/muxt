package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
)

func TestPackage(t *testing.T) {
	t.Run("valid execute call", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", `module example.com/testpkg

go 1.25.0
`)
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

		err := check.Package(pkg, nil, nil)
		require.NoError(t, err)
	})

	t.Run("type error in template", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", `module example.com/testpkg

go 1.25.0
`)
		writeFile(t, dir, "main.go", `package main

import (
	"html/template"
	"os"
)

type Data struct {
	Name string
}

var ts = template.Must(template.New("greeting").Parse(`+"`"+`Hello, {{.MissingField}}!`+"`"+`))

func main() {
	ts.ExecuteTemplate(os.Stdout, "greeting", Data{Name: "world"})
}
`)

		pkg := loadTestPackage(t, dir)

		err := check.Package(pkg, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MissingField not found")
	})

	t.Run("local var template", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", `module example.com/testpkg

go 1.25.0
`)
		writeFile(t, dir, "main.go", `package main

import (
	"html/template"
	"os"
)

type Item struct {
	Title string
}

func main() {
	ts := template.Must(template.New("item").Parse(`+"`"+`{{.Title}}`+"`"+`))
	ts.ExecuteTemplate(os.Stdout, "item", Item{})
}
`)

		pkg := loadTestPackage(t, dir)

		err := check.Package(pkg, nil, nil)
		require.NoError(t, err)
	})

	t.Run("local var type error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", `module example.com/testpkg

go 1.25.0
`)
		writeFile(t, dir, "main.go", `package main

import (
	"html/template"
	"os"
)

type Item struct {
	Title string
}

func main() {
	ts := template.Must(template.New("item").Parse(`+"`"+`{{.BadField}}`+"`"+`))
	ts.ExecuteTemplate(os.Stdout, "item", Item{})
}
`)

		pkg := loadTestPackage(t, dir)

		err := check.Package(pkg, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BadField not found")
	})
}

func TestPackageDirectory(t *testing.T) {
	t.Run("package with go files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "go.mod", `module example.com/testpkg

go 1.25.0
`)
		writeFile(t, dir, "main.go", `package main

func main() {}
`)

		pkg := loadTestPackage(t, dir)

		// packageDirectory is not exported, but we can verify it indirectly
		// by confirming the package loaded correctly with files in the expected directory
		require.NotEmpty(t, pkg.GoFiles)
		assert.Equal(t, dir, filepath.Dir(pkg.GoFiles[0]))
	})
}

func TestAnalyzePackage(t *testing.T) {
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

	var entries []check.Entry
	var nodeTypes []string
	_, err := check.AnalyzePackage(pkg, func(e check.Entry) check.NodeInspectorFunc {
		entries = append(entries, e)
		return func(info check.NodeTypeInfo) {
			if info.Type != nil {
				nodeTypes = append(nodeTypes, info.Type.String())
			}
		}
	})
	require.NoError(t, err)

	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "greeting", e.TemplateName)
	assert.Contains(t, e.DataType.String(), "Data")
	require.NotNil(t, e.Templates)
	assert.NotNil(t, e.Templates.Lookup("greeting"), "resolved set should resolve the template name")
	assert.NotNil(t, e.Tree)

	// {{.Name}} resolved to string and was reported through the per-node hook.
	assert.Contains(t, nodeTypes, "string")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	require.NoError(t, err)
}

func loadTestPackage(t *testing.T, dir string) *packages.Package {
	t.Helper()
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns |
			packages.NeedEmbedFiles | packages.NeedImports | packages.NeedModule,
		Dir: dir,
	}, ".")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	for _, e := range pkgs[0].Errors {
		t.Fatalf("package load error: %v", e)
	}
	return pkgs[0]
}
