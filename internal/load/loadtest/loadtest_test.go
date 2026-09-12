package loadtest_test

import (
	"path/filepath"
	"slices"
	"testing"

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

	pkg, sets, err := load.TemplateSets(dir, pl, []string{"templates", "inline"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Types.Path() != "example.com/server" {
		t.Errorf("package %s, want example.com/server", pkg.Types.Path())
	}
	for _, set := range sets {
		if set.Err != nil {
			t.Fatalf("%s: %v", set.Variable, set.Err)
		}
	}

	if sets[0].Set.Lookup("page") == nil {
		t.Error("templates does not hold the page ParseFS read")
	}
	definition, ok := sets[0].Definitions.FindDefinition("page")
	if !ok || definition.Define.Filename != filepath.Join(dir, "page.gohtml") {
		t.Errorf("page defined at %+v, want in page.gohtml", definition.Define)
	}
	if got := len(sets[0].Calls); got != 1 {
		t.Errorf("%d ExecuteTemplate calls, want the one Render makes", got)
	}
	var names []string
	for _, tmpl := range sets[1].Set.Templates() {
		names = append(names, tmpl.Name())
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"inline", "note"}) {
		t.Errorf("inline holds %q, want inline and note", names)
	}
}
