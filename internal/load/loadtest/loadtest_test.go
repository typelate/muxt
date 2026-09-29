package loadtest_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/load/loadtest"
)

func TestPackageLoadsTemplates(t *testing.T) {
	dir := t.TempDir()
	pl := loadtest.Package(t, dir, "example.com/server", map[string]string{
		"server.go": `package server

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))

var inline = template.Must(template.New("inline").Delims("[[", "]]").Parse(` + "`[[define \"note\"]][[.]][[end]]`" + `))

func Render(w io.Writer, name string) error {
	return templates.ExecuteTemplate(w, "page", name)
}
`,
		"page.gohtml": `{{define "page"}}<p>{{.}}</p>{{end}}`,
	})

	pkg, err := load.Package(dir, pl, []string{"templates", "inline"})
	require.NoError(t, err)
	assert.Equal(t, "example.com/server", pkg.Types.Path(), "package path")
	require.Len(t, pkg.Variables, 2)
	templates, inline := pkg.Variables[0], pkg.Variables[1]

	t.Run("ParseFS reads the embedded page", func(t *testing.T) {
		assert.NotNil(t, templates.Set.Lookup("page"), "templates does not hold the page ParseFS read")
		definition, ok := templates.Definitions["page"]
		assert.True(t, ok, "page has a definition")
		assert.Equal(t, filepath.Join(dir, "page.gohtml"), definition.Define.Filename, "page defined at %+v, want in page.gohtml", definition.Define)
	})

	t.Run("the one ExecuteTemplate call Render makes", func(t *testing.T) {
		assert.Len(t, templates.Calls, 1)
	})

	t.Run("an inline template with custom delimiters", func(t *testing.T) {
		var names []string
		for _, tmpl := range inline.Set.Templates() {
			names = append(names, tmpl.Name())
		}
		slices.Sort(names)
		assert.Equal(t, []string{"inline", "note"}, names, "inline holds inline and note")
	})
}
