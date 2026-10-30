package asteval

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/muxt/internal/template"
)

func TestParseTrees(t *testing.T) {
	for _, tt := range []struct {
		name  string
		text  string
		trees map[string]string
	}{
		{
			name:  "a top-level template",
			text:  `<p>{{.}}</p>`,
			trees: map[string]string{"t": `<p>{{.}}</p>`},
		},
		{
			name:  "defines beside the top-level template",
			text:  `{{define "a"}}A{{end}}{{define "b"}}B{{end}}`,
			trees: map[string]string{"t": `{{define "a"}}A{{end}}{{define "b"}}B{{end}}`, "a": `A`, "b": `B`},
		},
		{
			name:  "a define of the top-level name replaces a top-level template holding only defines",
			text:  `{{define "t"}}T{{end}}`,
			trees: map[string]string{"t": `T`},
		},
		{
			name:  "a block is a tree of its own",
			text:  `<main>{{block "b" .}}B{{end}}</main>`,
			trees: map[string]string{"t": `<main>{{block "b" .}}B{{end}}</main>`, "b": `B`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trees, err := ParseTrees("t", tt.text, "", "", nil, true)
			require.NoError(t, err)
			got := make(map[string]string, len(trees))
			for name, tree := range trees {
				got[name] = tree.Root.String()
			}
			assert.Equal(t, tt.trees, got, "ParseTrees(%q)", tt.text)
		})
	}
}

func TestParseTreesKeywordsGiveWayToFunctions(t *testing.T) {
	hasBreak := func(functions map[string]*types.Signature) bool {
		trees, err := ParseTrees("t", `{{range .}}{{break}}{{end}}`, "", "", functions, true)
		require.NoError(t, err)
		found := false
		template.Walk(trees["t"], func(_ *template.Tree, n template.Node) {
			if _, ok := n.(*template.BreakNode); ok {
				found = true
			}
		})
		return found
	}
	assert.True(t, hasBreak(nil), "break is a keyword without a function of that name")
	assert.False(t, hasBreak(map[string]*types.Signature{"break": nil}), "break is a function call once a function of that name is registered")
}

func TestParseTreesRedefinition(t *testing.T) {
	_, err := ParseTrees("t", `{{block "a" .}}x{{end}}{{define "a"}}y{{end}}`, "", "", nil, true)
	assert.EqualError(t, err, `template: t:1: template: multiple definition of template "a"`)
	trees, err := ParseTrees("t", `{{define "a"}}x{{end}}{{define "a"}}{{end}}`, "", "", nil, true)
	require.NoError(t, err)
	assert.Equal(t, "x", trees["a"].Root.String(), "a later empty define gives way")
}

func TestParseTreesText(t *testing.T) {
	// A text/template set's text has no HTML structure: a '<' in JSON
	// and an HTML comment a control structure crosses are text.
	text := `{"a": "<x", "n": {{.N}}}{{if .N}}<!-- {{end}} -->`
	trees, err := ParseTrees("t", text, "", "", nil, false)
	require.NoError(t, err, "ParseTrees(%q, text)", text)
	assert.Equal(t, text, trees["t"].Root.String())
	assert.False(t, trees["t"].HTMLMode, "HTMLMode")
	template.Walk(trees["t"], func(_ *template.Tree, n template.Node) {
		if _, ok := n.(*template.HTMLCommentNode); ok {
			t.Errorf("ParseTrees(%q, text) built an HTML comment", text)
		}
	})
}
