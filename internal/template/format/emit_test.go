package format_test

import (
	"bytes"
	htmltemplate "html/template"
	"testing"
	textparse "text/template/parse"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template/format"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
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
		missing     bool // a stylesheet of the package is unreadable
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
			name: "L2 unknown element in a branch leaves the branch as written",
			src:  "<main>\n{{if .A}}\n<div class=\"c\">\n<h2>x</h2>\n</div>\n{{else}}\n<p>n</p>\n{{end}}\n</main>",
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
			name: "blank lines collapse to one in collapse mode",
			src:  "<div>\n\n\n<p>x</p>\n</div>",
			want: "<div>\n\n\t<p>x</p>\n</div>",
		},
		{
			name: "blank lines before a doc comment collapse to one",
			src:  "<p></p>{{define \"a\"}}<p>a</p>{{end}}\n\n\n{{/* doc */ -}}\n{{define \"b\"}}<p>b</p>{{end}}",
			want: "<p></p>{{define \"a\"}}<p>a</p>{{end}}\n\n{{/* doc */ -}}\n{{define \"b\"}}<p>b</p>{{end}}",
		},
		{
			name: "blank lines in prose collapse to one",
			src:  "<p>a\n\n\nb</p>",
			want: "<p>a\n\n\tb</p>",
		},
		{
			name: "blank lines with spaces collapse to one empty line",
			src:  "<p>a\n  \n\t\n \nb</p>",
			want: "<p>a\n\n\tb</p>",
		},
		{
			name: "blank CRLF lines collapse to one",
			src:  "<div>\r\n\r\n\r\n<p>x</p>\r\n</div>",
			want: "<div>\n\n\t<p>x</p>\n</div>",
		},
		{
			name: "one blank line is never removed",
			src:  "<div>\n<p>x</p>\n\n</div>",
			want: "<div>\n\t<p>x</p>\n\n</div>",
		},
		{
			name: "blank lines in a trimmed run collapse to one",
			src:  "<div>\n{{- .A}}\n\n\n{{- .B}}\n</div>",
			want: "<div>\n\t{{- .A}}\n\n\t{{- .B}}\n</div>",
		},
		{
			name: "blank lines under pre-line are exact",
			src:  "<div style=\"white-space:pre-line\">a\n\n\n\t\tb</div>",
			want: "<div style=\"white-space:pre-line\">a\n\n\n\tb</div>",
		},
		{
			name: "blank lines inside pre are exact",
			src:  "<pre>a\n\n\nb</pre>",
		},
		{
			name: "final blank lines collapse to one",
			src:  "<p>x</p>\n\n\n",
			want: "<p>x</p>\n\n",
		},
		// L5: no token edits outside actions; action interiors are
		// printed from the parse tree (TestActionFormat).
		{name: "L5 backtick block name", src: "<div>{{block `a b` .}}x{{end}}</div>"},
		{name: "L5 void slash", src: "<input value='{{.ID}}'/><br/><br />"},
		{name: "L5 trim markers kept", src: "{{define \"x\" -}}{{template \"y\"}}{{- end}}<p></p>"},
		{name: "L5 action spacing normalized", src: "<p>{{ if  .A }}x{{ end }}{{ .X }}</p>", want: "<p>{{if .A}}x{{end}}{{.X}}</p>"},
		{name: "L5 closing tag case and space", src: "<DIV>x</div >"},
		{name: "L5 nested else-if not merged", src: "<p>{{if .A}}a{{else}}{{if .B}}b{{end}}{{end}}</p>"},
		{name: "L5 chained else-if kept", src: "<p>{{if .A}}a{{ else if .B }}b{{end}}</p>", want: "<p>{{if .A}}a{{else if .B}}b{{end}}</p>"},
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
			name: "unclosed span does not block an implied li end",
			src:  "<ul>\n<li>a<span>b\n<li>c\n</ul>",
			want: "<ul>\n\t<li>a<span>b\n\t<li>c\n</ul>",
		},
		{
			name: "unclosed span does not block an implied p end",
			src:  "<div>\n<p>a<span>b\n<p>c\n</div>",
			want: "<div>\n\t<p>a<span>b\n\t<p>c\n</div>",
		},
		{
			name: "unclosed div does not block an implied li end",
			src:  "<ul>\n<li>a<div>b\n<li>c\n</ul>",
			want: "<ul>\n\t<li>a<div>b\n\t<li>c\n</ul>",
		},
		{
			name: "unclosed section blocks an implied li end",
			src:  "<ul>\n<li>a<section>b\n<li>c\n</ul>",
			want: "<ul>\n\t<li>a<section>b\n\t\t\t<li>c\n</ul>",
		},
		{
			name: "unclosed span blocks an implied option end",
			src:  "<select>\n<option>a<span>b\n<option>c\n</select>",
			want: "<select>\n\t<option>a<span>b\n\t\t\t<option>c\n</select>",
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
		{
			name: "textarea on one line is reindented",
			src:  "<form>\n    <textarea>x  </textarea>\n</form>",
			want: "<form>\n\t<textarea>x  </textarea>\n</form>",
		},
		{
			name: "textarea over several lines left whole",
			src:  "<form>\n    <textarea>\nx  \n</textarea>\n    </form>",
			want: "<form>\n    <textarea>\nx  \n</textarea>\n</form>",
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
		{
			name: "attribute continuation indented with spaces only gets tabs",
			src:  "<div>\n<a href=\"x\"\n    title=\"y\">z</a>\n</div>",
			want: "<div>\n\t<a href=\"x\"\n\t\ttitle=\"y\">z</a>\n</div>",
		},
		{
			name: "attribute continuation with mixed indent gets tabs",
			src:  "<div>\n<a href=\"x\"\n\t \ttitle=\"y\">z</a>\n</div>",
			want: "<div>\n\t<a href=\"x\"\n\t\ttitle=\"y\">z</a>\n</div>",
		},
		{name: "attribute spacing on one line as written", src: "<a  href=\"x\"   class=\"y\" >z</a>"},
		{
			name: "attribute value line break exact",
			src:  "<div>\n<p title=\"a\n  b\">x</p>\n</div>",
			want: "<div>\n\t<p title=\"a\n  b\">x</p>\n</div>",
		},
		// Line endings and file edges.
		// Line endings are normalized to LF everywhere in the file, before
		// parsing: CR LF and a lone CR become LF (HTML §13.2.3.5 does the
		// same before tokenizing, so the page is unchanged).
		{
			name: "CRLF becomes LF",
			src:  "<div>\r\n<p>x</p>\r\n</div>\r\n",
			want: "<div>\n\t<p>x</p>\n</div>\n",
		},
		{
			name: "CR alone becomes LF",
			src:  "<div>\r<p>x</p>\r</div>",
			want: "<div>\n\t<p>x</p>\n</div>",
		},
		{
			name: "mixed line endings become LF",
			src:  "<div>\r\n<p>x</p>\n<p>y</p>\r</div>\n\r",
			want: "<div>\n\t<p>x</p>\n\t<p>y</p>\n</div>\n\n",
		},
		{
			name: "lone CR file without an element becomes LF",
			src:  "a\rb\r",
			want: "a\nb\n",
		},
		{
			name: "CRLF in pre becomes LF",
			src:  "<div>\r\n<pre>\r\nx\r\n  y\r\n</pre>\r\n</div>\r\n",
			want: "<div>\n<pre>\nx\n  y\n</pre>\n</div>\n",
		},
		{
			name: "CRLF in textarea becomes LF",
			src:  "<div>\r\n<textarea>\r\na\r\n  b</textarea>\r\n</div>\r\n",
			want: "<div>\n<textarea>\na\n  b</textarea>\n</div>\n",
		},
		{
			name: "CRLF in script becomes LF",
			src:  "<script>\r\nlet a = 1;\r\n  let b = 2;\r\n</script>\r\n",
			want: "<script>\nlet a = 1;\n  let b = 2;\n</script>\n",
		},
		{
			name: "CRLF in an attribute value becomes LF",
			src:  "<div>\r\n<p title=\"a\r\n  b\rc\">x</p>\r\n</div>",
			want: "<div>\n\t<p title=\"a\n  b\nc\">x</p>\n</div>",
		},
		{
			name: "CRLF in an HTML comment becomes LF",
			src:  "<div>\r\n<!-- a\r\n  b -->\r\n</div>",
			want: "<div>\n\t<!-- a\n  b -->\n</div>",
		},
		{
			name: "CRLF in a template comment becomes LF",
			src:  "<div>\r\n{{/* a\r\nb */}}\r\n</div>",
			want: "<div>\n\t{{/* a\n\tb */}}\n</div>",
		},
		{
			name: "CRLF in a muxt:fmt off region becomes LF",
			src:  "<div>\r\n{{/* muxt:fmt off */}}\r\n    <p>x</p>\r\n{{/* muxt:fmt on */}}\r\n</div>\r\n",
			want: "<div>\n\t{{/* muxt:fmt off */}}\n    <p>x</p>\n{{/* muxt:fmt on */}}\n</div>\n",
		},
		{
			name: "CRLF in a file kept whole by muxt:fmt off becomes LF",
			src:  "{{/* muxt:fmt off */}}\r\n<div>\r\n    <p>x</p>\r\n</div>\r\n",
			want: "{{/* muxt:fmt off */}}\n<div>\n    <p>x</p>\n</div>\n",
		},
		{
			name: "CRLF in a file left as written becomes LF",
			src:  "<div>\r\n  <p>x</p>\r\n<!x>\r\n</div>",
			want: "<div>\n  <p>x</p>\n<!x>\n</div>",
		},
		{
			name: "CR, space, LF under pre-line keeps both breaks",
			src:  "<div style=\"white-space:pre-line\">a\r \nb</div>",
			want: "<div style=\"white-space:pre-line\">a\n\n\tb</div>",
		},
		{
			name: "CR, space, LF under collapse is two breaks",
			src:  "<p>a\r \nb</p>",
			want: "<p>a\n\n\tb</p>",
		},
		{
			name: "trailing spaces before a break stripped",
			src:  "<div>  \n<p>x</p>\t\n</div>",
			want: "<div>\n\t<p>x</p>\n</div>",
		},
		{
			name: "trailing spaces in prose stripped",
			src:  "<p>x  \ny</p>",
			want: "<p>x\n\ty</p>",
		},
		{
			name: "trailing spaces before CRLF stripped",
			src:  "<div> \r\n<p>x</p>\r\n</div>",
			want: "<div>\n\t<p>x</p>\n</div>",
		},
		{
			name: "trailing spaces under pre-line stripped",
			src:  "<div style=\"white-space:pre-line\">a  \n\tb</div>",
			want: "<div style=\"white-space:pre-line\">a\n\tb</div>",
		},
		{
			name: "trailing spaces in a trimmed run stripped",
			src:  "<div>\n{{- .A}}  \n{{- .B}}\n</div>",
			want: "<div>\n\t{{- .A}}\n\t{{- .B}}\n</div>",
		},
		{name: "trailing spaces inside pre exact", src: "<pre>x  \ny</pre>"},
		{name: "trailing spaces before a comment have no break to strip", src: "<p>x\t{{/*\n*/}}y</p>"},
		{name: "trailing spaces in an attribute value exact", src: "<p title=\"a  \nb\">x</p>"},
		{
			name: "trailing spaces on the last line stripped",
			src:  "<p>x</p>  \n",
			want: "<p>x</p>\n",
		},
		{name: "trailing spaces without a final newline kept", src: "<p>x</p>  "},
		{name: "no final newline", src: "<p>x</p>"},
		{name: "final blank lines kept", src: "<p>x</p>\n\n"},
		{
			name: "first line indentation has no line break to reindent",
			src:  "\t\t<div>\n\t\t\t\t<p>x</p>\n\t\t</div>",
			want: "\t\t<div>\n\t<p>x</p>\n</div>",
		},
		// Task 11 (trial t7): consistency within a list, nested defines,
		// multi-line comments and actions.
		{
			name:    "list with an as-written gap: trimmed gaps as written too",
			src:     "{{define \"GET / Index()\" -}}\n{{- if .Err -}}\n<!DOCTYPE html>\n<html>\n<head><link rel=\"stylesheet\" href=\"/app.css\"></head>\n<body><p>x</p></body>\n</html>\n{{- end}}\n{{- end}}",
			missing: true,
		},
		{
			name:    "one preserved gap and several trimmed gaps: all as written",
			src:     "<p>x</p>\n{{define \"x\" -}}\n  {{- template \"a\" .}}\n  {{- template \"b\" .}}\n  {{template \"c\" .}}\n{{- end}}",
			missing: true,
		},
		{
			name:    "no preserved gap: trimmed gaps reindent",
			src:     "<p>x</p>\n{{define \"x\" -}}\n  {{- template \"a\" .}}\n  {{- template \"b\" .}}\n{{- end}}",
			want:    "<p>x</p>\n{{define \"x\" -}}\n{{- template \"a\" .}}\n{{- template \"b\" .}}\n{{- end}}",
			missing: true,
		},
		{
			name: "unsure element in a branch: the whole branch as written, its siblings reindent",
			src:  "<main>\n  <p>a</p>\n{{if .A -}}\n  <hr>\n<div class=\"c\">\n<h2>x</h2>\n</div>\n{{- else -}}\n  <p>n</p>\n{{- end}}\n  <p>b</p>\n</main>",
			want: "<main>\n\t<p>a</p>\n{{if .A -}}\n  <hr>\n<div class=\"c\">\n<h2>x</h2>\n</div>\n{{- else -}}\n  <p>n</p>\n{{- end}}\n\t<p>b</p>\n</main>",
		},
		{
			name: "a list left as written leaves its items' subtrees as written",
			src:  "{{define \"x\"}}\n\t<div class=\"c\">\n  {{- template \"l\" .}}\n\t</div>\n\t{{if .A -}}\n    {{template \"i\" . -}}\n\t{{end}}\n  <section\n      id=\"s\">\n<p>y</p>\n  </section>\n{{end}}",
		},
		{
			name: "with whose body is left as written keeps its keyword lines",
			src:  "{{define \"x\" -}}\n{{- if .Err}}{{template \"e\" .}}{{else}}\n{{- with .Result}}\n<div class=\"k\">\n<p>x</p>\n</div>\n{{- end}}\n{{- end}}\n{{- end}}",
		},
		{
			name:    "a frozen branch freezes the branch bodies around it up to the define body",
			src:     "{{define \"POST /m M()\" -}}\n{{- if .Err}}{{.E}}\n{{- else}}{{.R}}\n{{- range .Tabs}}\n{{- template \"wb\" .}}\n{{- if .Path}}\n<hx-partial hx-target='#t'>{{template \"t\" .}}</hx-partial>\n{{- end}}\n{{- end}}\n{{- end}}\n{{- end}}",
			missing: true,
		},
		{
			name: "a frozen branch chain in an element is an island: its siblings reindent",
			src:  "<div>\n  <main>\n  <p>a</p>\n{{- if .A}}\n{{- range .L}}\n<div class=\"c\">\n<b>x</b>\n</div>\n{{- end}}\n{{- end}}\n  </main>\n</div>",
			want: "<div>\n\t<main>\n\t\t<p>a</p>\n{{- if .A}}\n{{- range .L}}\n<div class=\"c\">\n<b>x</b>\n</div>\n{{- end}}\n{{- end}}\n\t</main>\n</div>",
		},
		{
			name: "an unsure multi-line element freezes its list wherever it starts",
			src:  "<section><div class=\"code\">\n  keep\n</div>\n<p>x</p>\n</section>",
		},
		{
			name: "script in head is an island: its siblings reindent",
			src:  "<head>\n<meta>\n<script>\nx\n</script>\n<link>\n</head>",
			want: "<head>\n\t<meta>\n<script>\nx\n</script>\n\t<link>\n</head>",
		},
		{
			name: "pre is an island: its siblings reindent",
			src:  "<div>\n<p>a</p>\n<pre>\nx\n</pre>\n<p>b</p>\n</div>",
			want: "<div>\n\t<p>a</p>\n<pre>\nx\n</pre>\n\t<p>b</p>\n</div>",
		},
		{
			name: "an unsure element leaves its list as written",
			src:  "<div>\n<p>a</p>\n<div class=\"k\">\n<p>x</p>\n</div>\n<p>b</p>\n</div>",
		},
		{
			name: "multi-line else is printed on one line",
			src:  "<div>\n{{if .A}}\n<p>a</p>\n    {{else\n}}\n<p>b</p>\n{{end}}\n</div>",
			want: "<div>\n\t{{if .A}}\n\t\t<p>a</p>\n\t{{else}}\n\t\t<p>b</p>\n\t{{end}}\n</div>",
		},
		{
			name: "multi-line else if is laid out one argument per line",
			src:  "<div>\n{{if .A}}\n<p>a</p>\n    {{else if and\n .B .C}}\n<p>b</p>\n{{end}}\n</div>",
			want: "<div>\n\t{{if .A}}\n\t\t<p>a</p>\n\t{{else if and\n\t\t.B\n\t\t.C\n\t}}\n\t\t<p>b</p>\n\t{{end}}\n</div>",
		},
		{
			name: "multi-line end is printed on one line",
			src:  "<div>\n{{range .L}}\n<p>a</p>\n    {{end\n}}\n</div>",
			want: "<div>\n\t{{range .L}}\n\t\t<p>a</p>\n\t{{end}}\n</div>",
		},
		{
			name: "define in an element is at column 0",
			src:  "<body>\n{{define \"y\"}}\n<p>a</p>\n{{end}}\n</body>",
		},
		{
			name: "indented define in an element moves to column 0",
			src:  "<div>\n\t\t{{define \"y\"}}\n\t<p>a</p>\n\t{{end}}\n\t<p>b</p>\n</div>",
			want: "<div>\n{{define \"y\"}}\n<p>a</p>\n{{end}}\n\t<p>b</p>\n</div>",
		},
		{
			name: "multi-line comment continuation lines shift with its first line",
			src:  "{{define \"x\"}}\n\t<p>a</p>\n\t{{- /* one\n\t       two */ -}}\n\t<p>b</p>\n{{end}}",
			want: "{{define \"x\"}}\n<p>a</p>\n{{- /* one\n       two */ -}}\n<p>b</p>\n{{end}}",
		},
		{
			name: "multi-line comment indented deeper keeps its hanging alignment",
			src:  "<div>\n{{/* one\n   two\n\n   three */}}\n</div>",
			want: "<div>\n\t{{/* one\n\t   two\n\n\t   three */}}\n</div>",
		},
		{
			name: "multi-line comment that cannot shift exactly is as written, its siblings reindent",
			src:  "<div>\n\t\t{{/* one\n two */}}\n\t\t<p>x</p>\n\t\t</div>",
			want: "<div>\n\t\t{{/* one\n two */}}\n\t<p>x</p>\n</div>",
		},
		{
			name: "multi-line action is printed on one line",
			src:  "<div>\n    {{template \"a\"\n\t.}}\n  <p>x</p>\n</div>",
			want: "<div>\n\t{{template \"a\" .}}\n\t<p>x</p>\n</div>",
		},
		{
			name: "multi-line branch pipeline is laid out one argument per line",
			src:  "<div>\n    {{if and\n\t.A .B}}\n\t\t\t<p>x</p>\n    {{end}}\n</div>",
			want: "<div>\n\t{{if and\n\t\t.A\n\t\t.B\n\t}}\n\t\t<p>x</p>\n\t{{end}}\n</div>",
		},
		{
			name: "action with a raw string spanning lines keeps its opening line",
			src:  "<div>\n    {{printf `a\nb`\n\t.X}}\n  <p>x</p>\n</div>",
			want: "<div>\n    {{printf `a\nb`\n\t.X}}\n\t<p>x</p>\n</div>",
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
			name: "muxt:fmt preserve first in the file keeps only the next node",
			src:  "{{/* muxt:fmt preserve */}}\n<span>a</span>\n<div>\n<p>\nb\n</p>\n</div>\n",
			want: "{{/* muxt:fmt preserve */}}\n<span>a</span>\n<div>\n\t<p>\n\t\tb\n\t</p>\n</div>\n",
		},
		{
			name: "muxt:fmt off first in the file without on keeps the file",
			src:  "{{/* muxt:fmt off */}}\n<span>a</span>\n<div>\n<p>\nb\n</p>\n</div>\n",
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
			if tt.missing {
				opts = append(opts, format.Styles(analysis.WhitespaceOptions{MissingSheets: true}))
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
			var sheets []analysis.Stylesheet
			if tt.css != "" {
				sheets = []analysis.Stylesheet{{Filename: "a.css", Text: tt.css}}
			}
			checkRendering(t, tt.src, got, tt.left, tt.right, sheets)
		})
	}
}

// checkRendering is the rendering oracle (the one FuzzFormatEquivalent
// runs) for one emit row: every template of src and of out, executed
// with each fuzz dot, renders the same page. A template the input
// cannot execute, or whose page the oracle cannot read, is skipped.
func checkRendering(t *testing.T, src, out, left, right string, sheets []analysis.Stylesheet) {
	t.Helper()
	parse := func(text string) *htmltemplate.Template {
		tmpl, err := htmltemplate.New("t.gohtml").Delims(left, right).Parse(text)
		if err != nil {
			t.Fatalf("html/template parse: %v\n%q", err, text)
		}
		return tmpl
	}
	in, formatted := parse(src), parse(out)
	for _, tmpl := range in.Templates() {
		name := tmpl.Name()
		if tmpl.Tree == nil || textparse.IsEmptyTree(tmpl.Tree.Root) {
			continue
		}
		for _, dot := range []map[string]any{
			{},
			{"Show": true, "X": "x", "Items": []any{"a", "b"}, "A": true, "B": true, "L": []any{"a", "b"}, "State": "clean"},
		} {
			want, err := execute(in, name, dot)
			if err != nil {
				continue
			}
			if _, err := rendering.Tokens(want, rendering.Body, sheets); err != nil {
				continue
			}
			got, err := execute(formatted, name, dot)
			if err != nil {
				t.Fatalf("output template %q does not execute with %v: %v", name, dot, err)
			}
			diff, err := rendering.Diff(want, got, rendering.Body, sheets)
			if err != nil {
				t.Fatalf("rendering.Diff of %q with %v: %v", name, dot, err)
			}
			if diff != "" {
				t.Errorf("rendering of %q changed with %v: %s\ninput rendered:  %q\noutput rendered: %q", name, dot, diff, want, got)
			}
		}
	}
}

func execute(tmpl *htmltemplate.Template, name string, dot map[string]any) (string, error) {
	clone, err := tmpl.Clone()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := clone.ExecuteTemplate(&buf, name, dot); err != nil {
		return "", err
	}
	return buf.String(), nil
}
