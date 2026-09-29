package muxt

import (
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkTemplateCommands(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		want     []string
	}{
		{name: "actions in order", template: `{{.A}}{{.B}}`, want: []string{".A true", ".B true"}},
		{name: "pipeline commands", template: `{{.A | .B}}`, want: []string{".A true", ".B true"}},
		{name: "parenthesised pipeline", template: `{{.F (.A) .B}}`, want: []string{".F (.A) .B true", ".A true"}},
		{name: "chained pipeline", template: `{{(.A).B}}`, want: []string{"(.A).B true", ".A true"}},
		{name: "if keeps dot", template: `{{if .A}}{{.B}}{{else}}{{.C}}{{end}}`, want: []string{".A true", ".B true", ".C true"}},
		{name: "with rebinds dot in the body only", template: `{{with .A}}{{.B}}{{else}}{{.C}}{{end}}`, want: []string{".A true", ".B false", ".C true"}},
		{name: "range rebinds dot in the body only", template: `{{range .A}}{{.B}}{{else}}{{.C}}{{end}}`, want: []string{".A true", ".B false", ".C true"}},
		{name: "template passing dot", template: `{{template "inner" .}}`, want: []string{". true", ".Inner true"}},
		{name: "template without an argument", template: `{{template "inner"}}`, want: []string{".Inner false"}},
		{name: "template with another argument", template: `{{template "inner" .A}}`, want: []string{".A true", ".Inner false"}},
		{name: "template argument is walked where it is written", template: `{{with .A}}{{template "inner" .}}{{end}}`, want: []string{".A true", ". false", ".Inner false"}},
		{name: "a template calling itself is walked once", template: `{{template "page" .}}`, want: []string{". true", ". true"}},
		{name: "unknown template", template: `{{template "missing" .}}{{.A}}`, want: []string{". true", ".A true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, page := pageIn(t, tt.template)
			_, err := ts.Parse(`{{define "inner"}}{{.Inner}}{{end}}`)
			require.NoError(t, err)
			var got []string
			walkTemplateCommands(page.Tree.Root, ts, make(map[string]bool), true, func(cmd *parse.CommandNode, dot bool) bool {
				got = append(got, cmd.String()+" "+boolWord(dot))
				return false
			})
			assert.Equal(t, tt.want, got, "walkTemplateCommands(%s) visited", tt.template)
		})
	}
}

func TestWalkTemplateCommandsStopsAtTheFirstAccepted(t *testing.T) {
	ts, page := pageIn(t, `{{.A}}{{if .B}}{{.C}}{{end}}{{.D}}`)
	var visited []string
	found := walkTemplateCommands(page.Tree.Root, ts, make(map[string]bool), true, func(cmd *parse.CommandNode, _ bool) bool {
		visited = append(visited, cmd.String())
		return cmd.String() == ".C"
	})
	assert.True(t, found, "walkTemplateCommands reported the accepted command")
	assert.Equal(t, []string{".A", ".B", ".C"}, visited, "walkTemplateCommands visited")
}

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
