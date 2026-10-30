package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template/lex"
)

// TestHTMLParseLenientTags covers markup that text/template and
// html/template parse but that is not a tag the HTML parser can build an
// element from. The parser reads such markup as text with actions: it
// parses, round-trips, builds no element for it, and records no
// diagnostic.
func TestHTMLParseLenientTags(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		elements []string // tag names of the elements the parser builds, in source order
	}{
		{name: "less than in text", input: `<p>1 < 2</p>`, elements: []string{"p"}},
		{name: "heart", input: `I <3 templates`},
		{name: "arrow", input: `back <- here`},
		{name: "less than at end", input: `a <`},
		{name: "empty end tag", input: `a</>b`},
		{name: "end tag without name", input: `</ >`},
		{name: "action as tag name", input: `<{{.Tag}}>tag</{{.Tag}}>`},
		{name: "action in tag name", input: `<h{{.N}}>heading</h{{.N}}>`},
		{name: "action in attribute name", input: `<div data-{{.Name}}="x">attribute name</div>`},
		{name: "comment in attributes", input: `<div {{/* c */}} class="a">comment in tag</div>`},
		{name: "trimmed comment in attributes", input: `<div class="b" {{- /* why */ -}}>trimmed comment in tag</div>`},
		{name: "comment in attribute value", input: `<div class="{{/* c */}}a">x</div>`},
		{name: "action before equals", input: `<div {{.Name}}="x">a</div>`},
		{name: "equals without name", input: `<div ="x">a</div>`},
		{name: "empty unquoted value", input: `<p id=>x</p>`},
		{name: "slash in attributes", input: `<a / b>x</a>`},
		{name: "attributes in end tag", input: `<p>a</p id>`, elements: []string{"p"}},
		{name: "action in end tag", input: `<p>a</p {{.Name}}>`, elements: []string{"p"}},
		{name: "unclosed start tag", input: `<a`},
		{name: "unclosed start tag with attribute", input: `<div class="x"`},
		{name: "unterminated attribute value", input: `<div title="x`},
		{name: "unclosed end tag", input: `<p>a</p`, elements: []string{"p"}},
		{name: "unclosed comment", input: `a<!--z`},
		{name: "unclosed doctype", input: `a<!DOCTYPE`},
		{name: "comment in raw text", input: `<script>{{/* c */}}a</script>`, elements: []string{"script"}},
		{name: "trimmed comment in raw text", input: `<textarea> {{- /* c */ -}} </textarea>`, elements: []string{"textarea"}},
		{name: "unclosed raw text end tag", input: `<script>a</script`, elements: []string{"script"}},
		{name: "raw text end tag with attribute", input: `<script>a</script x>b`, elements: []string{"script"}},
		{name: "raw text end tag ended by control flow", input: `{{if .X}}<script>a</script{{end}}`, elements: []string{"script"}},
		{name: "degraded element inside element", input: `<section><div data-{{.Name}}="x">a</div>b</section>`, elements: []string{"section"}},
		{name: "degraded element around element", input: `<div data-{{.Name}}="x"><div>a</div></div>`, elements: []string{"div"}},
		{name: "start tag ended by control flow", input: `{{if .X}}<div class="x"{{end}}>`},
		{name: "start tag ended by else", input: `{{if .X}}<div class="x"{{else}}<div class="y"{{end}}>`},
		{name: "start tag ended by define end", input: `{{define "open"}}<div class="x"{{end}}`},
		{name: "well formed tag with control flow in attributes", input: `<input {{if .X}}checked{{else}}disabled{{end}}>`, elements: []string{"input"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			trees := map[string]*Tree{test.name: tree}
			for name, tr := range Extract(tree) {
				trees[name] = tr
			}
			var elements []string
			for _, tr := range trees {
				Walk(tr, func(_ *Tree, n Node) {
					if elem, ok := n.(*HTMLElementNode); ok {
						elements = append(elements, elem.TagName)
					}
				})
			}
			if len(trees) == 1 {
				assert.Equal(t, test.input, tree.Root.String(), "round trip")
			}
			assert.Equal(t, test.elements, elements, "elements")
			assert.Empty(t, tree.Diagnostics, "diagnostics")
		})
	}
}

// TestHTMLParseLenientTagsText checks the nodes a degraded tag becomes:
// its source is text, its actions are actions, and the comment stays a
// comment.
func TestHTMLParseLenientTagsText(t *testing.T) {
	tree, err := ParseTree("test.gohtml", "x", `<h{{.N}} {{/* c */}}>a`, nil, lex.NewOptions())
	require.NoError(t, err)
	requireASTEqual(t, tree.Root, &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
		&TextNode{Text: []byte("<h")},
		&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"N"}}}}}}},
		&TextNode{Text: []byte(" ")},
		&CommentNode{Text: "/* c */"},
		&TextNode{Text: []byte(">a")},
	}}}})
}
