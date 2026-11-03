package format_test

import (
	"testing"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template/format"
)

// TestEmit pins the reindent-only emitter (v2 spec §3 items 1–4, 8, 9):
// only the whitespace after the last line break of a reindentable run
// changes, to one tab per depth level; every other byte is the source's.
func TestEmit(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want string // "" means unchanged
		// known says the author CSS is known (css is all of it); without
		// it an element with a class or id is unknown.
		known       bool
		css         string
		left, right string
	}{
		// Branches: keywords at the enclosing depth, bodies +1.
		{
			name: "else-if chain with inline content reindents keywords only",
			src:  "{{define \"x\"}}\n<div>\n\t\t{{- if eq .State \"clean\"}}<span>clean</span>\n{{- else if eq .State \"conflicts\"}}<span>c</span>\n    {{- else}}<span>n</span>\n{{- end}}\n</div>\n{{end}}",
			want: "{{define \"x\"}}\n<div>\n\t{{- if eq .State \"clean\"}}<span>clean</span>\n\t{{- else if eq .State \"conflicts\"}}<span>c</span>\n\t{{- else}}<span>n</span>\n\t{{- end}}\n</div>\n{{end}}",
		},
		{
			name: "range body",
			src:  "<ul>\n{{range .L}}\n<li>{{.}}</li>\n{{end}}\n</ul>",
			want: "<ul>\n\t{{range .L}}\n\t\t<li>{{.}}</li>\n\t{{end}}\n</ul>",
		},
		{
			name: "with and else",
			src:  "<div>\n{{with .A}}\n<p>a</p>\n{{else}}\n<p>b</p>\n{{end}}\n</div>",
			want: "<div>\n\t{{with .A}}\n\t\t<p>a</p>\n\t{{else}}\n\t\t<p>b</p>\n\t{{end}}\n</div>",
		},
		{
			name: "else then a nested if is not a chain",
			src:  "<div>\n{{if .A}}\na\n{{else}}{{if .B}}\nb\n{{end}}{{end}}\n</div>",
			want: "<div>\n\t{{if .A}}\n\t\ta\n\t{{else}}{{if .B}}\n\t\t\tb\n\t\t{{end}}{{end}}\n</div>",
		},
		{
			name: "break inside nested branches",
			src:  "<div>\n{{range .L}}\n{{if .}}\n{{break}}\n{{end}}\n{{end}}\n</div>",
			want: "<div>\n\t{{range .L}}\n\t\t{{if .}}\n\t\t\t{{break}}\n\t\t{{end}}\n\t{{end}}\n</div>",
		},
		{
			name: "branch depth change does not leak past end",
			src:  "{{if .A}}<div>{{end}}  <p>x</p>",
		},
		{
			name: "unclosed element in a branch closes at end",
			src:  "<main>\n{{if .A}}\n<div>\n{{end}}\n<p>x</p>\n</main>",
			want: "<main>\n\t{{if .A}}\n\t\t<div>\n\t{{end}}\n\t<p>x</p>\n</main>",
		},
		// Defines, blocks, calls.
		{
			name: "define body at column 0",
			src:  "{{define \"x\"}}\n\t\t<div>\n\t\t\t<p>x</p>\n\t\t</div>\n{{end}}",
			want: "{{define \"x\"}}\n<div>\n\t<p>x</p>\n</div>\n{{end}}",
		},
		{
			name: "full page define unchanged",
			src:  "{{define \"x\" -}}<!DOCTYPE html>\n<html>\n<head>\n\t<title>T</title>\n</head>\n<body>\n\t<p>x</p>\n</body>\n</html>\n{{end}}",
		},
		{
			name: "html adds no depth, head and body children +1",
			src:  "<!DOCTYPE html>\n  <html lang=\"en\">\n  <head>\n<meta charset=\"utf-8\">\n  </head>\n  <body>\n<main>\n<p>x</p>\n</main>\n  </body>\n  </html>\n",
			want: "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n\t<meta charset=\"utf-8\">\n</head>\n<body>\n\t<main>\n\t\t<p>x</p>\n\t</main>\n</body>\n</html>\n",
		},
		{
			name: "template call is transparent",
			src:  "<div>\n{{template \"a\" .}}\n    <p>x</p>\n</div>",
			want: "<div>\n\t{{template \"a\" .}}\n\t<p>x</p>\n</div>",
		},
		{
			name: "block is transparent",
			src:  "<main>\n{{block \"b\" .}}\n<p>x</p>\n{{end}}\n</main>",
			want: "<main>\n\t{{block \"b\" .}}\n\t<p>x</p>\n\t{{end}}\n</main>",
		},
		{
			name: "head with a template call",
			src:  "<html>\n<head>\n<title>T</title>{{template \"head\"}}\n</head>\n<body>\n<main><h1>x</h1></main>\n</body>\n</html>",
			want: "<html>\n<head>\n\t<title>T</title>{{template \"head\"}}\n</head>\n<body>\n\t<main><h1>x</h1></main>\n</body>\n</html>",
		},
		// L1: trimmed runs are free but never joined.
		{
			name: "L1 trimmed runs keep their lines",
			src:  "{{define \"x\" -}}\n  {{- if .A}}<b>a</b> {{- else}}b {{- end}}\n{{- end}}",
			want: "{{define \"x\" -}}\n{{- if .A}}<b>a</b> {{- else}}b {{- end}}\n{{- end}}",
		},
		{
			name: "L1 action before doctype stays on the define line",
			src:  "{{define \"x\" -}} {{.Status 404}}<!DOCTYPE html>\n<html><body><p>x</p></body></html> {{- end}}",
		},
		{
			name: "L1 trimmed with body indents",
			src:  "<main>\n\t\t{{- with .A -}}\n\t\t{{- template \"a\" . -}}\n\t\t{{- end -}}\n</main>",
			want: "<main>\n\t{{- with .A -}}\n\t\t{{- template \"a\" . -}}\n\t{{- end -}}\n</main>",
		},
		{
			name: "trimmed run without a break is as written",
			src:  "<p>{{.A}}  {{- .B}}</p>",
		},
		{
			name: "trimmed run keeps only its line breaks",
			src:  "<div>\n{{- .A}}\n  \n  {{- .B}}\n</div>",
			want: "<div>\n\t{{- .A}}\n\n\t{{- .B}}\n</div>",
		},
		// L2: unknown elements are left whole.
		{
			name: "L2 unknown element left whole",
			src:  "<main>\n<div class=\"c\">\n<h2>x</h2>\n</div>\n</main>",
		},
		{
			name: "L2 unknown element keeps its column, parent closer at parent depth",
			src:  "<main>\n    <div class=\"c\">\n  <h2>x</h2>\n    </div>\n    </main>",
			want: "<main>\n    <div class=\"c\">\n  <h2>x</h2>\n    </div>\n</main>",
		},
		{
			name: "L2 unknown element in a branch",
			src:  "<main>\n{{if .A}}\n<div class=\"c\">\n<h2>x</h2>\n</div>\n{{else}}\n<p>n</p>\n{{end}}\n</main>",
			want: "<main>\n\t{{if .A}}\n<div class=\"c\">\n<h2>x</h2>\n</div>\n\t{{else}}\n\t\t<p>n</p>\n\t{{end}}\n</main>",
		},
		{
			name: "L2 one-line unknown element is reindented and blank line kept",
			src:  "<main>\n<p class=\"a\">x</p>\n\n<p>y</p>\n</main>",
			want: "<main>\n\t<p class=\"a\">x</p>\n\n\t<p>y</p>\n</main>",
		},
		{
			name:  "known preserve element left whole",
			src:   "<style>.k{white-space:pre-wrap}</style>\n<div>\n  <div class=\"k\">\n   x\n</div>\n</div>",
			want:  "<style>.k{white-space:pre-wrap}</style>\n<div>\n  <div class=\"k\">\n   x\n</div>\n</div>",
			known: true,
		},
		{
			name:  "known CSS class reindents",
			src:   "<main>\n<div class=\"c\">\n<h2>x</h2>\n</div>\n</main>",
			want:  "<main>\n\t<div class=\"c\">\n\t\t<h2>x</h2>\n\t</div>\n</main>",
			known: true,
			css:   ".c{color:red}",
		},
		// L3: head.
		{
			name: "L3 head children",
			src:  "<head>\n<meta charset=\"utf-8\">\n</head>",
			want: "<head>\n\t<meta charset=\"utf-8\">\n</head>",
		},
		// L4: blank lines between top-level items.
		{
			name: "L4 blank lines between defines kept",
			src:  "{{define \"a\"}}\n<p>a</p>\n{{end}}\n\n{{define \"b\"}}\n<p>b</p>\n{{end}}\n{{/* doc */ -}}\n{{define \"c\"}}\n<p>c</p>\n{{end}}\n",
		},
		{
			name: "blank line count is exact in this task",
			src:  "<div>\n\n\n<p>x</p>\n</div>",
			want: "<div>\n\n\n\t<p>x</p>\n</div>",
		},
		// L5: no token edits.
		{name: "L5 backtick block name", src: "<div>{{block `a b` .}}x{{end}}</div>"},
		{name: "L5 void slash", src: "<input value='{{.ID}}'/><br/><br />"},
		{name: "L5 trim markers kept", src: "{{define \"x\" -}}{{template \"y\"}}{{- end}}<p></p>"},
		{name: "L5 action spacing", src: "<p>{{ if  .A }}x{{ end }}{{ .X }}</p>"},
		{name: "L5 closing tag case and space", src: "<DIV>x</div >"},
		{name: "L5 nested else-if not merged", src: "<p>{{if .A}}a{{else}}{{if .B}}b{{end}}{{end}}</p>"},
		{name: "L5 chained else-if kept", src: "<p>{{if .A}}a{{ else if .B }}b{{end}}</p>"},
		// L6: no split.
		{name: "L6 one-line block stays", src: "<div><p>e</p></div>"},
		{name: "L6 one-line define body stays", src: "{{define \"a\"}}<p>a</p>{{end}}"},
		// Optional end tags.
		{name: "optional li on one line", src: "<ul><li>a<li>b</ul>"},
		{
			name: "optional li end tags are implied",
			src:  "<ul>\n<li>a\n<li>b\n</ul>",
			want: "<ul>\n\t<li>a\n\t<li>b\n</ul>",
		},
		{
			name: "li with a nested list",
			src:  "<ul>\n<li>a\n<ul>\n<li>b\n</ul>\n<li>c\n</ul>",
			want: "<ul>\n\t<li>a\n\t\t<ul>\n\t\t\t<li>b\n\t\t</ul>\n\t<li>c\n</ul>",
		},
		{
			name: "dt and dd",
			src:  "<dl>\n<dt>a\n<dd>b\n<dt>c\n</dl>",
			want: "<dl>\n\t<dt>a\n\t<dd>b\n\t<dt>c\n</dl>",
		},
		{
			name: "table rows and cells",
			src:  "<table>\n<tr>\n<td>a\n<td>b\n<tr>\n<td>c\n</table>",
			want: "<table>\n\t<tr>\n\t\t<td>a\n\t\t<td>b\n\t<tr>\n\t\t<td>c\n</table>",
		},
		{
			name: "p closed by a block start tag",
			src:  "<div>\n<p>a\n<div>b</div>\n</div>",
			want: "<div>\n\t<p>a\n\t<div>b</div>\n</div>",
		},
		{
			name: "other unclosed element closes at the parent end tag",
			src:  "<div>\n<span>\n<b>x</b>\n</div>",
			want: "<div>\n\t<span>\n\t\t<b>x</b>\n</div>",
		},
		// Kind is fixed; CJK; pre.
		{
			name: "CJK segment break is indented only",
			src:  "<p>日本\n語</p>",
			want: "<p>日本\n\t語</p>",
		},
		{name: "spaces inside a line as written", src: "<p>a   b\t c</p>"},
		{name: "pre leading LF exact", src: "<pre>\n\n x</pre>"},
		{
			name: "pre on one line is reindented",
			src:  "<div>\n    <pre>x</pre>\n</div>",
			want: "<div>\n\t<pre>x</pre>\n</div>",
		},
		{
			name: "pre over several lines left whole",
			src:  "<div>\n    <pre>\nx\n</pre>\n    </div>",
			want: "<div>\n    <pre>\nx\n</pre>\n</div>",
		},
		{name: "script left whole", src: "<body>\n  <script>\n  var x = 1;\n</script>\n</body>"},
		{
			name: "pre-line keeps every line break",
			src:  "<div style=\"white-space:pre-line\">a\n\n\t\t\tb</div>",
			want: "<div style=\"white-space:pre-line\">a\n\n\tb</div>",
		},
		{
			name: "prose lines",
			src:  "<p>\nHello\n  <a href=\"#\">world</a>,\n    welcome.\n</p>",
			want: "<p>\n\tHello\n\t<a href=\"#\">world</a>,\n\twelcome.\n</p>",
		},
		{name: "accessible name never split", src: "<button><span>Un</span><span>read</span></button>"},
		// Comments.
		{name: "html comment between text", src: "<p>a<!--x-->b</p>"},
		{
			name: "html comment on its own line",
			src:  "<p>\na\n    <!--x-->\nb\n</p>",
			want: "<p>\n\ta\n\t<!--x-->\n\tb\n</p>",
		},
		{name: "template comment between text", src: "<p>a{{/*c*/}}b</p>"},
		// Attributes.
		{
			name: "attribute continuation keeps alignment spaces",
			src:  "<div>\n\t<a href=\"x\"\n\t   title=\"y\">z</a>\n</div>",
			want: "<div>\n\t<a href=\"x\"\n\t\t   title=\"y\">z</a>\n</div>",
		},
		{
			name: "attribute continuation of an unquoted value",
			src:  "<div>\n<a href=x\ntitle=y>z</a>\n</div>",
			want: "<div>\n\t<a href=x\n\t\ttitle=y>z</a>\n</div>",
		},
		{
			name: "closing bracket on its own line at the tag depth",
			src:  "<div>\n<a\n\thref=\"x\"\n    >z</a>\n</div>",
			want: "<div>\n\t<a\n\t\thref=\"x\"\n\t>z</a>\n</div>",
		},
		{name: "attribute spacing on one line as written", src: "<a  href=\"x\"   class=\"y\" >z</a>"},
		{
			name: "attribute value line break exact",
			src:  "<div>\n<p title=\"a\n  b\">x</p>\n</div>",
			want: "<div>\n\t<p title=\"a\n  b\">x</p>\n</div>",
		},
		// Line endings and file edges.
		{
			name: "CRLF kept",
			src:  "<div>\r\n<p>x</p>\r\n</div>\r\n",
			want: "<div>\r\n\t<p>x</p>\r\n</div>\r\n",
		},
		{
			name: "CR alone is a line break",
			src:  "<div>\r<p>x</p>\r</div>",
			want: "<div>\r\t<p>x</p>\r</div>",
		},
		{
			name: "trailing spaces before a break kept in this task",
			src:  "<div>  \n<p>x</p>\n</div>",
			want: "<div>  \n\t<p>x</p>\n</div>",
		},
		{name: "no final newline", src: "<p>x</p>"},
		{name: "final blank lines kept", src: "<p>x</p>\n\n"},
		{
			name: "first line indentation has no line break to reindent",
			src:  "\t\t<div>\n\t\t\t\t<p>x</p>\n\t\t</div>",
			want: "\t\t<div>\n\t<p>x</p>\n</div>",
		},
		// Directives and custom delimiters.
		{
			name: "muxt:fmt off region as written",
			src:  "<div>\n{{/* muxt:fmt off */}}\n    <p>x</p>\n{{/* muxt:fmt on */}}\n    <p>y</p>\n</div>",
			want: "<div>\n\t{{/* muxt:fmt off */}}\n    <p>x</p>\n{{/* muxt:fmt on */}}\n\t<p>y</p>\n</div>",
		},
		{
			name: "muxt:fmt preserve element as written",
			src:  "<div>\n{{/* muxt:fmt preserve */}}\n    <p>x</p>\n    <p>y</p>\n</div>",
			want: "<div>\n\t{{/* muxt:fmt preserve */}}\n    <p>x</p>\n\t<p>y</p>\n</div>",
		},
		{
			name: "custom delimiters",
			src:  "[[if .A]]\n<p>x</p>\n  [[- end]]",
			want: "[[if .A]]\n\t<p>x</p>\n[[- end]]",
			left: "[[", right: "]]",
		},
		{
			name: "unparsed markup leaves the file as written",
			src:  "<div>\n  <p>x</p>\n<!x>\n</div>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []format.Option
			if tt.known {
				var sheets []analysis.Stylesheet
				if tt.css != "" {
					sheets = []analysis.Stylesheet{{Filename: "a.css", Text: tt.css}}
				}
				opts = append(opts, format.Styles(analysis.WhitespaceOptions{Sheets: sheets, AuthorCSSKnown: true}))
			}
			if tt.left != "" {
				opts = append(opts, format.Delimiters(tt.left, tt.right))
			}
			want := tt.want
			if want == "" {
				want = tt.src
			}
			got, err := format.Source("t.gohtml", tt.src, opts...)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", tt.src, err)
			}
			if got != want {
				t.Errorf("Source(%q)\n got %q\nwant %q", tt.src, got, want)
			}
			again, err := format.Source("t.gohtml", got, opts...)
			if err != nil {
				t.Fatalf("Source(output) error: %v", err)
			}
			if again != got {
				t.Errorf("not idempotent: Source(%q) = %q", got, again)
			}
		})
	}
}
