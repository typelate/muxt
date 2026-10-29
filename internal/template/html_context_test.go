package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template/lex"
)

// TestHTMLContextFromAST verifies that our HTML AST provides sufficient
// structural information to determine the escaping context at each template
// action, matching what html/template's transition-based escaper infers.
//
// html/template uses a state machine over TextNode bytes to track context.
// Our AST makes context explicit: the position of an ActionNode in the tree
// (inside HTMLElementNode.Children, HTMLAttributeNode.Value,
// HTMLCommentNode.Children, etc.) directly encodes the context.
func TestHTMLContextFromAST(t *testing.T) {
	// ctx abbreviates an expected HTMLContext row.
	type ctx = HTMLContext

	cases := []struct {
		name  string
		input string
		want  []HTMLContext // one per action/template in document order
	}{
		{"action in text", `<p>{{.Name}}</p>`, []ctx{{State: "text", Element: "p"}}},
		{"action in attr value", `<a href="{{.URL}}">link</a>`, []ctx{{State: "attr", Element: "a", Attr: "href"}}},
		{"action in class attr", `<div class="{{.Class}}">text</div>`, []ctx{{State: "attr", Element: "div", Attr: "class"}}},
		{"action in comment", `<!-- {{.Comment}} -->`, []ctx{{State: "comment"}}},
		{"action in script", `<script>{{.Code}}</script>`, []ctx{{State: "text", Element: "script", JSContext: "expr"}}},
		{"action in style", `<style>{{.CSS}}</style>`, []ctx{{State: "text", Element: "style"}}},

		// Inside a <script>, a backtick template literal is a string
		// context, but `${` opens an expression that runs as code.
		{
			"action in template literal", "<script>var a = `{{.X}}`;</script>",
			[]ctx{{State: "text", Element: "script", JSContext: "tpl"}},
		},
		{
			"action inside interpolation", "<script>var a = `x${ {{.X}} }y`;</script>",
			[]ctx{{State: "text", Element: "script", JSContext: "expr"}},
		},
		{
			"action after interpolation closes", "<script>var a = `${ n } {{.X}}`;</script>",
			[]ctx{{State: "text", Element: "script", JSContext: "tpl"}},
		},
		{
			"action inside interpolation containing an object literal",
			"<script>var a = `${ {k: {{.X}} } }`;</script>",
			[]ctx{{State: "text", Element: "script", JSContext: "expr"}},
		},
		{
			"action in template literal nested in interpolation",
			"<script>var a = `${ f(`{{.X}}`) }`;</script>",
			[]ctx{{State: "text", Element: "script", JSContext: "tpl"}},
		},
		{
			name:  "actions at each depth of a nested interpolation",
			input: "<script>var a = `{{.A}}${ {{.B}} }{{.C}}`;</script>",
			want: []ctx{
				{State: "text", Element: "script", JSContext: "tpl"},
				{State: "text", Element: "script", JSContext: "expr"},
				{State: "text", Element: "script", JSContext: "tpl"},
			},
		},
		{
			// A branch is analyzed from the state at branch entry and must
			// not leak its brace accounting back to the enclosing scan.
			name:  "branch brace depth does not leak to following actions",
			input: "<script>var a = `${ {{if .C}}{a:1{{end}} } {{.X}}`;</script>",
			want: []ctx{
				{State: "text", Element: "script", JSContext: "tpl"},
			},
		},
		{"action in textarea", `<textarea>{{.Content}}</textarea>`, []ctx{{State: "text", Element: "textarea"}}},
		{"action in title", `<title>{{.Title}}</title>`, []ctx{{State: "text", Element: "title"}}},
		{"action in top-level text", `{{.X}}`, []ctx{{State: "text"}}},
		{"action in mixed attr value", `<div class="base {{.X}}">text</div>`, []ctx{{State: "attr", Element: "div", Attr: "class"}}},
		{"action in onclick attr", `<button onclick="{{.Handler}}">click</button>`, []ctx{{State: "attr", Element: "button", Attr: "onclick"}}},
		{"action in style attr", `<div style="{{.Style}}">text</div>`, []ctx{{State: "attr", Element: "div", Attr: "style"}}},
		{"action in data-* attr", `<div data-id="{{.ID}}">text</div>`, []ctx{{State: "attr", Element: "div", Attr: "data-id"}}},
		{"action in hx-get attr", `<div hx-get="{{.URL}}">text</div>`, []ctx{{State: "attr", Element: "div", Attr: "hx-get"}}},
		{"action in aria attr", `<input aria-invalid="{{.Invalid}}">`, []ctx{{State: "attr", Element: "input", Attr: "aria-invalid"}}},

		{
			name:  "multiple actions in one element",
			input: `<a href="{{.URL}}">{{.Label}}</a>`,
			want: []ctx{
				{State: "attr", Element: "a", Attr: "href"},
				{State: "text", Element: "a"},
			},
		},
		{
			name:  "multiple nested contexts",
			input: `<a href="{{.URL}}"><img src="{{.Src}}"></a><p>{{.Text}}</p>`,
			want: []ctx{
				{State: "attr", Element: "a", Attr: "href"},
				{State: "attr", Element: "img", Attr: "src"},
				{State: "text", Element: "p"},
			},
		},

		// actions inside control structures
		{
			"action in if in attr", `<div {{if .Active}}class="{{.Class}}"{{end}}>text</div>`,
			[]ctx{{State: "attr", Element: "div", Attr: "class"}},
		},
		{
			"action in range body", `<ul>{{range .Items}}<li><p>{{.}}</p></li>{{end}}</ul>`,
			[]ctx{{State: "text", Element: "p"}},
		},

		// actions inside define bodies
		{
			name:  "action in define body",
			input: `{{define "link"}}<a href="{{.URL}}">{{.Label}}</a>{{end}}`,
			want: []ctx{
				{State: "attr", Element: "a", Attr: "href"},
				{State: "text", Element: "a"},
			},
		},

		// {{template}} calls carry the same context annotations
		{"template in top-level text", `{{template "x" .}}`, []ctx{{State: "text"}}},
		{"template in element text", `<p>{{template "body" .}}</p>`, []ctx{{State: "text", Element: "p"}}},
		{"template in attr value", `<a href="{{template "url" .}}">link</a>`, []ctx{{State: "attr", Element: "a", Attr: "href"}}},
		{"template in comment", `<!-- {{template "info" .}} -->`, []ctx{{State: "comment"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := ParseTree("test.gohtml", tc.name, tc.input, nil, lex.NewOptions())
			require.NoError(t, err)
			AnnotateHTMLContexts(tree.Root)

			got := collectActionContexts(tree)
			require.Len(t, got, len(tc.want), "action context count")
			for i := range got {
				assert.Equal(t, tc.want[i], got[i], "context %d", i)
			}
		})
	}
}

// collectActionContexts returns the HTMLContext attached to every ActionNode
// and TemplateNode, in document order. Walk handles tree traversal so this
// stays aligned with the canonical AST shape.
func collectActionContexts(tree *Tree) []HTMLContext {
	var contexts []HTMLContext
	Walk(tree, func(_ *Tree, n Node) {
		switch n := n.(type) {
		case *ActionNode:
			if n.HTMLContext != nil {
				contexts = append(contexts, *n.HTMLContext)
			}
		case *TemplateNode:
			if n.HTMLContext != nil {
				contexts = append(contexts, *n.HTMLContext)
			}
		}
	})
	return contexts
}
