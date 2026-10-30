package format_test

import (
	"bytes"
	htmltemplate "html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format"
	"github.com/typelate/muxt/internal/template/lex"
)

func parse(t *testing.T, input string) template.Node {
	t.Helper()
	tree, err := template.ParseTree("test", "test", input, nil, lex.NewOptions())
	require.NoError(t, err)
	return tree.Root
}

func TestCustomDelimiters(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
		left  string
		right string
	}{
		{
			name:  "custom delimiters action",
			input: "<p><% .Name %></p>",
			left:  "<%",
			right: "%>",
			want:  "<p><%.Name%></p>",
		},
		{
			name:  "custom delimiters branch",
			input: "<% if .Show %><p>yes</p><% end %>",
			left:  "<%",
			right: "%>",
			want:  "<%if .Show%><p>yes</p><%end%>",
		},
		{
			name:  "custom delimiters define",
			input: `<% define "foo" %><p>hi</p><% end %>`,
			left:  "<%",
			right: "%>",
			want:  "<%define \"foo\" -%>\n\t<p>hi</p>\n<%- end%>",
		},
		{
			name:  "custom delimiters template call",
			input: `<td><% template "comp" .X %></td>`,
			left:  "<%",
			right: "%>",
			want:  "<td><%template \"comp\" .X%></td>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := template.ParseTree("test", "test", tt.input, nil, lex.Options{
				EmitComment: true,
				ParseHTML:   true,
				LeftDelim:   tt.left,
				RightDelim:  tt.right,
				BreakOK:     true,
				ContinueOK:  true,
			})
			require.NoError(t, err)
			got := format.Node(tree.Root, format.Delimiters(tt.left, tt.right))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdempotency(t *testing.T) {
	inputs := []string{
		"<div><p>hello</p></div>",
		`<div class="foo" id="bar"><p>text</p></div>`,
		`{{if .A}}<p>a</p>{{else}}<p>b</p>{{end}}`,
		`{{range .Items}}<li>{{.}}</li>{{end}}`,
		`{{define "foo"}}<p>content</p>{{end}}`,
		"<!DOCTYPE html><html><body><p>hi</p></body></html>",
		"<pre>  preserved  </pre>",
		"<script>var x = 1;</script>",
		`{{if .A -}}<p>a</p>{{end}}`,
		`{{range .Items -}}<li>{{.}}</li>{{end}}`,
		`{{define "foo" -}}<p>content</p>{{end}}`,
		`{{if .A -}}<p>a</p>{{else -}}<p>b</p>{{end}}`,
		"<div>\n\t<p>first</p>\n\n\t<p>second</p>\n</div>",
		"<pre>  line one\n\n  line two  </pre>",
		// Short elements with template calls stay inline
		`<td>{{template "comp" .X}}</td>`,
		`<th>1 Year<br>{{template "note" ""}}</th>`,
		// Short branch bodies stay inline
		`{{if .Show}}<p>yes</p>{{end}}`,
		`{{with .X}}<p>{{.Value}}</p>{{else}}<p>fallback</p>{{end}}`,
		`{{if .A}}<p>a</p>{{else if .B}}<p>b</p>{{else}}<p>c</p>{{end}}`,
		// Inline element spacing
		`<p><span>a</span> <span>b</span></p>`,
		// Trim markers with short inline bodies
		`{{- if .Label -}}{{.Label}}{{- else -}}Entity[{{.ID}}]{{- end -}}`,
		// Space between adjacent actions in inline content
		`<p>&copy; {{.Year}} {{.Title}}</p>`,
		// Inline prose flows with elements
		`<p>Visit <a href="#">link</a> for info.</p>`,
		`<p><strong>Notice:</strong> {{.Message}}</p>`,
		// Inline define body stays on one line
		`{{define "comp"}}{{.Value}}{{end}}`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			root1 := parse(t, input)
			first := format.Node(root1)

			root2 := parse(t, first)
			second := format.Node(root2)

			assert.Equal(t, first, second, "not idempotent")
		})
	}
}

func TestFormattingEdgeCases(t *testing.T) {
	data := map[string]any{
		"Title":   "Hello",
		"Show":    true,
		"A":       true,
		"B":       false,
		"X":       map[string]any{"Value": "inner"},
		"Items":   []map[string]any{{"Name": "alpha", "Active": true}},
		"Name":    "World",
		"Value":   "42",
		"Active":  true,
		"Done":    false,
		"URL":     "https://example.com",
		"Label":   "click",
		"Class":   "main",
		"ID":      7,
		"Year":    2026,
		"Message": "hello",
	}

	tests := []struct {
		name       string
		input      string
		normalizes bool // true when formatter normalizes whitespace (tab→space, strip-only-ws)
	}{
		// --- Inline text flow around elements ---
		{name: "text before inline element", input: `<p>Hello <em>world</em></p>`},
		{name: "text after inline element", input: `<p><em>Hello</em> world</p>`},
		{name: "text surrounding inline element", input: `<p>Visit <a href="#">here</a> now.</p>`},
		{name: "punctuation after inline element", input: `<p>See <a href="#">this</a>.</p>`},
		{name: "comma after inline element", input: `<p>Read <a href="#">more</a>, then continue.</p>`},
		{name: "multiple inline elements with text", input: `<p><em>Hello</em> and <strong>world</strong>!</p>`},
		{name: "nested inline elements", input: `<p><strong><em>bold italic</em></strong> text</p>`},

		// --- Adjacent actions ---
		{name: "adjacent actions no space", input: `<p>{{.Name}}{{.Value}}</p>`},
		{name: "adjacent actions with space", input: `<p>{{.Name}} {{.Value}}</p>`},
		{name: "action after text", input: `<p>Hello {{.Name}}</p>`},
		{name: "action before text", input: `<p>{{.Name}} is here</p>`},
		{name: "action between text", input: `<p>Hello {{.Name}} world</p>`},
		{name: "multiple actions with spaces", input: `<p>{{.Name}} {{.Value}} {{.Title}}</p>`},

		// --- Void elements ---
		{name: "br in text", input: `<p>Line 1<br>Line 2</p>`},
		{name: "adjacent br", input: `<p><br><br></p>`},
		{name: "br between text", input: `<p>before<br>after</p>`},
		{name: "img in text", input: `<p>See <img src="x.png" alt="pic"> here</p>`},
		{name: "adjacent void and inline", input: `<p><br><span>text</span></p>`},

		// --- Mixed content ---
		{name: "inline element then action", input: `<p><strong>Label:</strong> {{.Value}}</p>`},
		{name: "action then inline element", input: `<p>{{.Title}} <em>emphasized</em></p>`},
		{name: "text action element mixed", input: `<p>Copyright {{.Year}} <a href="#">{{.Name}}</a></p>`},

		// --- Block elements with inline children ---
		{name: "div with inline children", input: `<div><span>a</span> <span>b</span> <span>c</span></div>`},
		{name: "li with inline content", input: `<li><a href="#">{{.Name}}</a></li>`},
		{name: "td with mixed content", input: `<td>{{.Name}} ({{.Value}})</td>`},

		// --- Whitespace normalization (browser-equivalent, standard formatter behavior) ---
		{name: "multiple spaces between words", input: `<p>Hello     world</p>`, normalizes: true},
		{name: "newline between inline elements", input: "<p><span>a</span>\n<span>b</span></p>", normalizes: true},
		{name: "tab between inline elements", input: "<p><span>a</span>\t<span>b</span></p>", normalizes: true},
		{name: "space before closing tag", input: `<p>text </p>`, normalizes: true},
		{name: "space after opening tag", input: `<p> text</p>`, normalizes: true},

		// --- Long inline content (should NOT wrap prose) ---
		{name: "long paragraph with link", input: `<p>This is a sentence with <a href="#">a link</a> embedded in it that might be long enough to matter.</p>`},

		// --- Empty and minimal ---
		{name: "empty paragraph", input: `<p></p>`},
		{name: "whitespace only paragraph", input: `<p>   </p>`, normalizes: true},
		{name: "single space paragraph", input: `<p> </p>`, normalizes: true},
		{name: "empty inline element", input: `<p><span></span></p>`},

		// --- Control flow with inline content ---
		{name: "if with inline text", input: `<p>{{if .Show}}visible{{end}} always</p>`},
		{name: "if-else inline", input: `<p>{{if .Show}}yes{{else}}no{{end}} text</p>`},
		{name: "range producing inline", input: `<span>{{range .Items}}{{.Name}} {{end}}</span>`},

		// --- Prose content that exceeds line width ---
		{name: "medium paragraph", input: `<p>This is a medium length paragraph with some text.</p>`},
		{name: "paragraph with emphasis", input: `<p>This is a paragraph with <em>emphasized words</em> inside it.</p>`},
		{name: "paragraph just over 80 chars", input: `<p>This paragraph is written to be just slightly over eighty characters in total width.</p>`},

		// --- Indentation introduced in text content ---
		{name: "heading with text", input: `<h1>Page Title</h1>`},
		{name: "heading with action", input: `<h1>{{.Title}}</h1>`},
		{name: "li with text and action", input: `<li>Item: {{.Name}}</li>`},
		{name: "dd with prose", input: `<dd>A description of the term that is relatively short.</dd>`},

		// --- Source formatting variations (whitespace normalization expected) ---
		{name: "already indented paragraph", input: "<p>\n\tSome text\n</p>", normalizes: true},
		{name: "already indented with inline", input: "<p>\n\t<strong>Bold</strong> text\n</p>", normalizes: true},

		// --- Define semantic preservation ---
		{name: "inline define stays inline", input: `{{define "comp"}}{{.Value}}{{end}}`, normalizes: false},
		{name: "block define stays block", input: `{{define "header"}}<header><h1>{{.Title}}</h1></header>{{end}}`, normalizes: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse and format
			root1 := parse(t, tt.input)
			formatted := format.Node(root1)

			// Idempotency check
			root2 := parse(t, formatted)
			second := format.Node(root2)
			assert.Equal(t, formatted, second, "not idempotent\nformatted: %q\nsecond:    %q", formatted, second)

			// Render input template
			inputTmpl, err := htmltemplate.New("test").Parse(tt.input)
			require.NoError(t, err, "input is not valid html/template")
			var inputBuf bytes.Buffer
			require.NoError(t, inputTmpl.Execute(&inputBuf, data))

			// Render formatted template
			fmtTmpl, err := htmltemplate.New("test").Parse(formatted)
			require.NoError(t, err, "formatted output is not valid html/template: %s", formatted)
			var fmtBuf bytes.Buffer
			require.NoError(t, fmtTmpl.Execute(&fmtBuf, data))

			inputHTML := inputBuf.String()
			fmtHTML := fmtBuf.String()

			// Log for inspection
			if formatted != tt.input {
				t.Logf("input:     %s", tt.input)
				t.Logf("formatted: %s", formatted)
				t.Logf("rendered input:  %s", inputHTML)
				t.Logf("rendered fmt:    %s", fmtHTML)
			}

			// The rendered output should match (unless only whitespace normalization)
			if !tt.normalizes {
				assert.Equal(t, inputHTML, fmtHTML, "rendered output changed!\nformatted template: %s", formatted)
			}
		})
	}
}

func TestDefineSemanticPreservation(t *testing.T) {
	// Formatting a define must not change the rendered output of callers.
	// The bug: defineNode adds a leading newline+tab and a trailing newline
	// around the body, turning <wrap>inner</wrap> into <wrap>\n\tinner\n</wrap>
	// when the define is invoked.
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "inline action body",
			src:  `{{define "comp"}}{{.Value}}{{end}}` + "\n" + `<td>{{template "comp" .X}}</td>`,
		},
		{
			name: "block element body",
			src:  `{{define "wrap"}}<p>{{.Value}}</p>{{end}}` + "\n" + `<div>{{template "wrap" .X}}</div>`,
		},
		{
			name: "nested block elements",
			src:  `{{define "card"}}<header><h1>{{.Value}}</h1></header>{{end}}` + "\n" + `<div>{{template "card" .X}}</div>`,
		},
	}

	data := map[string]any{"X": map[string]any{"Value": "inner"}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parse(t, tt.src)
			formatted := format.Node(root)

			inputTmpl, err := htmltemplate.New("test").Parse(tt.src)
			require.NoError(t, err, "input is not valid html/template")
			var inputBuf bytes.Buffer
			require.NoError(t, inputTmpl.Execute(&inputBuf, data), "input template execution failed")

			fmtTmpl, err := htmltemplate.New("test").Parse(formatted)
			require.NoError(t, err, "formatted output is not valid html/template: %s", formatted)
			var fmtBuf bytes.Buffer
			require.NoError(t, fmtTmpl.Execute(&fmtBuf, data), "formatted template execution failed")

			assert.Equal(t, inputBuf.String(), fmtBuf.String(),
				"rendered HTML changed after formatting\ninput template:     %s\nformatted template: %s",
				tt.src, formatted)
		})
	}
}
