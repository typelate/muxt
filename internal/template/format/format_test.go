package format_test

import (
	"bytes"
	htmltemplate "html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
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
			// The body holds a block-level p and its boundary gaps are
			// reflowable, so it takes block layout (spec §6.4).
			name:  "custom delimiters branch",
			input: "<% if .Show %><p>yes</p><% end %>",
			left:  "<%",
			right: "%>",
			want:  "<%if .Show%>\n\t<p>yes</p>\n<%end%>",
		},
		{
			// The body's first and last items are block-level, so its
			// edge gaps are reflowable and block layout needs no trim
			// markers (spec §6.5). The formatter no longer forces them.
			name:  "custom delimiters define",
			input: `<% define "foo" %><p>hi</p><% end %>`,
			left:  "<%",
			right: "%>",
			want:  "<%define \"foo\"%>\n\t<p>hi</p>\n<%end%>",
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
		// Block layout follows the source (gofmt model): a line break
		// in a gap the browser drops keeps the list a block, and the
		// same list without it stays inline, however long its line.
		`{{define "x"}}<b>a</b>{{end}}` + "\n\n" + `{{define "y"}}<b>b</b>{{end}}`,
		`{{define "x"}}<b>` + strings.Repeat("a", 100) + `</b>{{end}}{{define "y"}}<b>b</b>{{end}}`,
		`<br>{{define "x"}}<p>` + strings.Repeat("a", 44) + `</p>{{end}}<br>`,
		// Adjacent defines in a block-layout list get the blank line
		// between them on the first pass, not only once the break is
		// in the source (found by fuzzing).
		`{{define "a"}}{{end}}{{define "b"}}{{end}}<p></p>0`,
		// A define body ending in an unclosed element stays inline: a
		// break after it would be its content next pass.
		"{{define \"0\"}}\n<A0>{{end}}0",
		// {{else}}{{with}} is not merged into {{else with}}, which the
		// template parser does not read in HTML input.
		"{{if .}}{{else}}{{with .}}{{end}}{{end}}<A>",
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

			// The rendered page should match (unless only whitespace
			// normalization). Block layout may add whitespace the browser
			// drops, so the check is the oracle's, not byte equality.
			if !tt.normalizes {
				diff, err := rendering.Diff(inputHTML, fmtHTML, rendering.Body, nil)
				require.NoError(t, err)
				assert.Empty(t, diff, "rendered output changed!\nformatted template: %s", formatted)
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
		{
			// V4: the body's edges are collapse-only, so the source line
			// breaks render as spaces next to the inline caller's text and
			// no trim marker may remove them.
			name: "inline body called inline",
			src:  "{{define \"x\"}}\n<span>{{.Value}}</span>\n{{end}}<p>x{{template \"x\" .X}}y</p>",
		},
		{
			// The edge spaces render next to the caller's text; block
			// layout would need markers that remove them, so the body
			// stays inline.
			name: "spaced edges called inline",
			src:  `{{define "x"}} <span>a</span><div>{{.Value}}</div><span>c</span> {{end}}<p>x{{template "x" .X}}y</p>`,
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

			diff, err := rendering.Diff(inputBuf.String(), fmtBuf.String(), rendering.Body, nil)
			require.NoError(t, err)
			assert.Empty(t, diff,
				"rendered page changed after formatting\ninput template:     %s\nformatted template: %s",
				tt.src, formatted)
		})
	}
}

func TestAttributesAndCRLF(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
	}{
		{
			name:  "attributes on one line stay on one line",
			input: `<div class="a" id="b" data-x="1" data-y="2">x</div>`,
			want:  `<div class="a" id="b" data-x="1" data-y="2">x</div>`,
		},
		{
			name:  "one long attribute is not wrapped",
			input: `<a href="https://example.com/` + strings.Repeat("a", 92) + `">x</a>`,
			want:  `<a href="https://example.com/` + strings.Repeat("a", 92) + `">x</a>`,
		},
		{
			name:  "attributes the source breaks go one per line",
			input: "<input type=\"text\"\n    name=\"x\"\nrequired>",
			want:  "<input type=\"text\"\n\tname=\"x\"\n\trequired>",
		},
		{
			name:  "attribute list re-indented one tab deeper than its tag",
			input: "<div>\n<p>\n<input type=\"text\"\n      name=\"x\"\n  required>\n</p>\n</div>",
			want:  "<div>\n\t<p>\n\t\t<input type=\"text\"\n\t\t\tname=\"x\"\n\t\t\trequired>\n\t</p>\n</div>",
		},
		{
			name:  "closing bracket on its own line stays there",
			input: "<input\n  type=\"text\"\n  name=\"x\"\n>",
			want:  "<input\n\ttype=\"text\"\n\tname=\"x\"\n>",
		},
		{
			name:  "spaces between attributes become one",
			input: `<div  class="a"   id="b"  >x</div>`,
			want:  `<div class="a" id="b">x</div>`,
		},
		{
			name:  "attribute value with a line break keeps its lines",
			input: "<div title=\"one\n  two\n\tthree\" id=\"x\">y</div>",
			want:  "<div title=\"one\n  two\n\tthree\" id=\"x\">y</div>",
		},
		{
			name:  "attribute values keep their source quoting",
			input: `<div a='x' b=y c="z" d={{.X}} e='{{.Y}}'>w</div>`,
			want:  `<div a='x' b=y c="z" d={{.X}} e='{{.Y}}'>w</div>`,
		},
		{
			name:  "attributes joined without a space stay joined",
			input: `<div a="x"b="y" {{.X}}{{.Y}}=1>w</div>`,
			want:  `<div a="x"b="y" {{.X}}{{.Y}}=1>w</div>`,
		},
		{
			name:  "unquoted value that runs into an action",
			input: `<A 0={ {{0}}0>`,
			want:  `<A 0={ {{0}}0>`,
		},
		{
			name:  "control flow in an open tag is written from source",
			input: `<A {{if0}}{{end}}0><A>`,
			want:  `<A {{if0}}{{end}}0><A>`,
		},
		{
			name:  "branch attributes are written from source",
			input: `<div  {{if .A}}class="a"{{else}}  id="b"{{end}}   data-x>w</div>`,
			want:  `<div  {{if .A}}class="a"{{else}}  id="b"{{end}}   data-x>w</div>`,
		},
		{
			name:  "comment action in an open tag",
			input: `<a {{/* c */}} b>x</a>`,
			want:  `<a {{/* c */}} b>x</a>`,
		},
		{
			name:  "self-closing slash kept on a non-void element",
			input: `<A0 x />`,
			want:  `<A0 x />`,
		},
		{
			name:  "CRLF is written as LF outside opaque regions",
			input: "<div>\r\n<p>a</p>\r\n</div>\r\n<pre>\r\nx\r\n</pre>\r\n",
			want:  "<div>\n\t<p>a</p>\n</div>\n<pre>\r\nx\r\n</pre>\r\n",
		},
		{
			name:  "CRLF in a broken attribute list",
			input: "<input type=\"text\"\r\n  name=\"x\">\r\n",
			want:  "<input type=\"text\"\n\tname=\"x\">\r\n",
		},
		{
			name:  "final run kept: three line feeds",
			input: "<p>a</p>\n\n\n",
			want:  "<p>a</p>\n\n\n",
		},
		{
			name:  "final run kept: none",
			input: "<p>a</p>",
			want:  "<p>a</p>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := format.Source("t.gohtml", tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "Source(%q)", tt.input)
			again, err := format.Source("t.gohtml", got)
			require.NoError(t, err)
			assert.Equal(t, got, again, "not idempotent")
		})
	}
}

// TestCSSEntryPoints documents where author CSS comes from: Node and
// Source read only the CSS that Styles names and the template's own
// <style> elements. Without either, an element with a class or id is
// unknown (decision 1) and its content is left as written.
func TestCSSEntryPoints(t *testing.T) {
	const input = `<div class="c"><p>a</p></div>`
	t.Run("Node without CSS", func(t *testing.T) {
		assert.Equal(t, input, format.Node(parse(t, input)))
	})
	t.Run("Source with author CSS known", func(t *testing.T) {
		got, err := format.Source("t.gohtml", input, format.Styles(analysis.WhitespaceOptions{AuthorCSSKnown: true}))
		require.NoError(t, err)
		assert.Equal(t, "<div class=\"c\">\n\t<p>a</p>\n</div>", got)
	})
	t.Run("Source with a style element", func(t *testing.T) {
		got, err := format.Source("t.gohtml", "<style>.d{}</style>"+input)
		require.NoError(t, err)
		assert.Equal(t, "<style>.d{}</style><div class=\"c\">\n\t<p>a</p>\n</div>", got)
	})
}

// TestUnparsedMarkup checks markup the template parser reads as text but
// the browser reads as a tag. When it changes how the rest of the file
// tokenizes (a raw text start tag with a < in an attribute name, a bogus
// comment) the file is left as written; any other unread tag leaves only
// its parent, and the ancestors up to a clean block, as written.
func TestUnparsedMarkup(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
	}{
		{name: "start tag with < in its attributes", input: "<teXtAreA </teXtAreA><A  >0", want: "<teXtAreA </teXtAreA><A  >0"},
		{name: "bogus end tag", input: "<div>a</ b>  <p>  c</p></div>", want: "<div>a</ b>  <p>  c</p></div>"},
		{name: "comment action first", input: "{{/* c */}}<div>a</ b>  <p>  c</p></div>", want: "{{/* c */}}<div>a</ b>  <p>  c</p></div>"},
		{name: "unmatched end tag makes its parent unsure", input: "<div><p>a</p></b></div><div><p>b</p></div>", want: "<div><p>a</p></b></div><div>\n\t<p>b</p>\n</div>"},
		{name: "unread start tag keeps its parent as written", input: "<ul>\n<li{{if .A}} class=\"a\"{{end}}>x</li>\n</ul><div><p>b</p></div>", want: "<ul>\n<li{{if .A}} class=\"a\"{{end}}>x</li>\n</ul><div>\n\t<p>b</p>\n</div>"},
		{name: "less-than in prose is text", input: "<div>a < b<p>c</p></div>", want: "<div>\n\ta < b\n\t<p>c</p>\n</div>"},
		{name: "tags in script are raw text", input: "<script>a <b</script><div><p>c</p></div>", want: "<script>a <b</script><div>\n\t<p>c</p>\n</div>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := format.Source("t.gohtml", tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "Source(%q)", tt.input)
		})
	}
}
