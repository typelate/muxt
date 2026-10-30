package template_test

import (
	"testing"
	"text/template/parse"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/html"
)

// TestTreeRootStart states that a defined tree's root starts where
// text/template's parser started it. text/template's lexer dropped the
// whitespace a {{define ... -}} or {{block ... -}} trims, so its root list
// began at the first byte the template writes, or at the action after the
// trimmed space. The want is text/template's own root position.
func TestTreeRootStart(t *testing.T) {
	for _, tt := range []struct {
		name, text, template string
	}{
		{name: "trimmed space before an action", template: "a", text: "{{- define \"a\" -}}\n\t{{- .X -}}\n{{- end -}}"},
		{name: "trimmed space before text", template: "b", text: "{{define \"b\" -}}\n  hi {{.Y}}{{end}}"},
		{name: "untrimmed space", template: "c", text: "{{define \"c\"}}\n  hi{{end}}"},
		{name: "only trimmed space", template: "d", text: "{{define \"d\" -}}  \n {{end}}"},
		{name: "space trimmed from both sides", template: "e", text: "{{define \"e\" -}}  \n {{- end}}"},
		{name: "block", template: "f", text: "{{block \"f\" . -}}\n x{{end}}"},
		{name: "trimmed space before markup", template: "h", text: "{{define \"h\" -}}\n  <p>{{.X}}</p>{{end}}"},
		{name: "no space", template: "g", text: "{{define \"g\" -}}x{{end}}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trees, err := parse.Parse("p", tt.text, "", "", map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			want := int(trees[tt.template].Root.Position())

			t.Run("text", func(t *testing.T) {
				tmpl, err := template.New[any]("p").Parse(tt.text)
				if err != nil {
					t.Fatal(err)
				}
				tree, ok := tmpl.FindTree(tt.template)
				if !ok {
					t.Fatalf("template %q not found", tt.template)
				}
				if got := int(tree.RootStart()); got != want {
					t.Errorf("RootStart() = %d, want %d (text/template), in %q", got, want, tt.text)
				}
			})
			t.Run("html", func(t *testing.T) {
				ts, err := html.New("p").Parse(tt.text)
				if err != nil {
					t.Fatal(err)
				}
				tree := ts.Lookup(tt.template).Tree
				if got := int(tree.RootStart()); got != want {
					t.Errorf("RootStart() = %d, want %d (text/template), in %q", got, want, tt.text)
				}
			})
		})
	}
}
