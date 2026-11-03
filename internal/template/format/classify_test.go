package format

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		src, css string
		want     string
	}{
		{
			src:  "<div><span>a</span><span>b</span>\n</div>",
			want: "⟨c⟩<div>⟨c⟩<span>⟨c⟩a⟨c⟩</span>⟨c⟩<span>⟨c⟩b⟨c⟩</span>⟨C⟩</div>⟨c⟩",
		},
		{
			src:  "<div>\n<b>Hi</b>{{.Name}}\n</div>",
			want: "⟨c⟩<div>⟨C⟩<b>⟨c⟩Hi⟨c⟩</b>⟨c⟩{{.Name}}⟨C⟩</div>⟨c⟩",
		},
		{
			src:  "<div>\n x{{if .Show}}<b>y</b>{{end}}\n</div>",
			want: "⟨c⟩<div>⟨C⟩x⟨c⟩{{if .Show}}⟨c⟩<b>⟨c⟩y⟨c⟩</b>⟨c⟩{{end}}⟨C⟩</div>⟨c⟩",
		},
		{
			src:  "<ul>{{range .}}<li>x</li>{{end}}</ul>",
			want: "⟨c⟩<ul>⟨c⟩{{range .}}⟨c⟩<li>⟨c⟩x⟨c⟩</li>⟨c⟩{{end}}⟨c⟩</ul>⟨c⟩",
		},
		{
			src:  "<p>a {{- .X}}</p>",
			want: "⟨c⟩<p>⟨c⟩a⟨T⟩{{- .X}}⟨c⟩</p>⟨c⟩",
		},
		{
			src:  "<div>a<div hidden></div>b\n</div>",
			want: "⟨c⟩<div>⟨c⟩a⟨c⟩<div hidden></div>⟨c⟩b⟨C⟩</div>⟨c⟩",
		},
		{
			src:  "<p>a <span class=\"x\">\n b </span> c</p>",
			want: "⟨c⟩<p>⟨c⟩a⟨C⟩<span class=\"x\">\n b </span>⟨C⟩c⟨c⟩</p>⟨c⟩",
		},
		{
			src:  "{{define \"x\"}}\n<span>a</span>\n{{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩<span>⟨c⟩a⟨c⟩</span>⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "{{define \"x\"}}\n<div>a</div>\n{{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩<div>⟨c⟩a⟨c⟩</div>⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "<pre>\n  a  </pre>",
			want: "⟨c⟩<pre>\n  a  </pre>⟨c⟩",
		},
		{
			src:  "<p style=\"white-space:pre-line\">a \n b</p>",
			want: "⟨c⟩<p style=\"white-space:pre-line\">⟨b⟩a⟨B⟩b⟨b⟩</p>⟨c⟩",
		},
		{
			src:  "<div>a{{/* c */}}b</div>",
			want: "⟨c⟩<div>⟨c⟩a⟨c⟩{{/* c */}}⟨c⟩b⟨c⟩</div>⟨c⟩",
		},
		{
			src:  "Dear {{.Name}},\n\nThanks",
			want: "Dear⟨P⟩{{.Name}},⟨P⟩Thanks",
		},
		// A break ends the range next to what follows it.
		{
			src:  "<p>{{range .}}<b>s</b>{{break}}{{end}} y</p>",
			want: "⟨c⟩<p>⟨c⟩{{range .}}⟨c⟩<b>⟨c⟩s⟨c⟩</b>⟨c⟩{{break}}⟨c⟩{{end}}⟨C⟩y⟨c⟩</p>⟨c⟩",
		},
		// A continue meets the next iteration's start.
		{
			src:  "<p>{{range .}}<b>s</b>{{continue}}{{end}}</p>",
			want: "⟨c⟩<p>⟨c⟩{{range .}}⟨c⟩<b>⟨c⟩s⟨c⟩</b>⟨c⟩{{continue}}⟨c⟩{{end}}⟨c⟩</p>⟨c⟩",
		},
		// Iteration n ends next to iteration n+1.
		{
			src:  "<p>{{range .}}<b>x</b>{{end}}</p>",
			want: "⟨c⟩<p>⟨c⟩{{range .}}⟨c⟩<b>⟨c⟩x⟨c⟩</b>⟨c⟩{{end}}⟨c⟩</p>⟨c⟩",
		},
		// br is an edge; an atomic box is opaque outside and has edges
		// inside; display: contents children join the parent's flow.
		{
			src:  "<div>a<br>b<span style=\"display:inline-block\">c</span><slot>d</slot></div>",
			want: "⟨c⟩<div>⟨c⟩a⟨c⟩<br>⟨c⟩b⟨c⟩<span style=\"display:inline-block\">⟨c⟩c⟨c⟩</span>⟨c⟩<slot>⟨c⟩d⟨c⟩</slot>⟨c⟩</div>⟨c⟩",
		},
		// A display: none element may be shown by a script, so it is
		// opaque from outside and no run next to it is reflowable; its
		// inside has no certain edges.
		{
			src:  "<div><div hidden> x </div></div>",
			want: "⟨c⟩<div>⟨c⟩<div hidden>⟨C⟩x⟨C⟩</div>⟨c⟩</div>⟨c⟩",
		},
		{
			src:  "<div>x <span hidden>y</span></div>",
			want: "⟨c⟩<div>⟨c⟩x⟨C⟩<span hidden>⟨c⟩y⟨c⟩</span>⟨c⟩</div>⟨c⟩",
		},
		{
			src:  "<div>\n<div hidden>y</div>\n</div>",
			want: "⟨c⟩<div>⟨C⟩<div hidden>⟨c⟩y⟨c⟩</div>⟨C⟩</div>⟨c⟩",
		},
		// An element whose display is unknown is treated the same way:
		// a conditional hidden changes only display.
		{
			src:  "<div>\n<div {{if .X}}hidden{{end}}>y</div>\n</div>",
			want: "⟨c⟩<div>⟨C⟩<div {{if .X}}hidden{{end}}>⟨c⟩y⟨c⟩</div>⟨C⟩</div>⟨c⟩",
		},
		{
			src:  "<p>{{if .X -}} a {{- end}}</p>",
			want: "⟨c⟩<p>⟨c⟩{{if .X -}}⟨T⟩a⟨T⟩{{- end}}⟨c⟩</p>⟨c⟩",
		},
		// A define body's edges are reflowable when every first (last)
		// item is block-level whatever the branch.
		{
			src:  "{{define \"x\"}} {{if .A}}<div>a</div>{{else}}<p>b</p>{{end}} {{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩{{if .A}}⟨c⟩<div>⟨c⟩a⟨c⟩</div>⟨c⟩{{else}}⟨c⟩<p>⟨c⟩b⟨c⟩</p>⟨c⟩{{end}}⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "{{define \"x\"}} {{if .A}}<div>a</div>{{end}} {{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩{{if .A}}⟨c⟩<div>⟨c⟩a⟨c⟩</div>⟨c⟩{{end}}⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "<table><tr><td>a</td> <td>b</td></tr></table>",
			want: "⟨c⟩<table>⟨c⟩<tr>⟨c⟩<td>⟨c⟩a⟨c⟩</td>⟨C⟩<td>⟨c⟩b⟨c⟩</td>⟨c⟩</tr>⟨c⟩</table>⟨c⟩",
		},
		{
			src:  "<p>a <em class=\"k\"> b </em> c</p>",
			css:  ".k{white-space:pre}",
			want: "⟨c⟩<p>⟨c⟩a⟨C⟩<em class=\"k\"> b </em>⟨C⟩c⟨c⟩</p>⟨c⟩",
		},
		{
			src:  "<p>a <em class=\"k\"> b </em> c</p>",
			css:  ".k{display:block}",
			want: "⟨c⟩<p>⟨c⟩a⟨C⟩<em class=\"k\">⟨C⟩b⟨C⟩</em>⟨C⟩c⟨c⟩</p>⟨c⟩",
		},
		{
			src:  "<p> a  b </p>",
			want: "⟨c⟩<p>⟨C⟩a⟨C⟩b⟨C⟩</p>⟨c⟩",
		},
		{
			src:  "<p><b>x</b> y <b>z</b></p>",
			want: "⟨c⟩<p>⟨c⟩<b>⟨c⟩x⟨c⟩</b>⟨C⟩y⟨C⟩<b>⟨c⟩z⟨c⟩</b>⟨c⟩</p>⟨c⟩",
		},
	} {
		if got := annotate(t, tt.src, tt.css); got != tt.want {
			t.Errorf("annotate(%q) =\n%s\nwant\n%s", tt.src, got, tt.want)
		}
	}
}

// annotate returns src with each non-empty whitespace run replaced by a
// marker of its class (⟨C⟩ collapse, ⟨P⟩ preserved, ⟨T⟩ trimmed, ⟨B⟩
// preserve-breaks) and each empty gap marked ⟨c⟩ or ⟨b⟩. Runs inside opaque subtrees and gaps of lists with no items get
// no marker. With css empty, no author CSS is known.
func annotate(t *testing.T, src, css string) string {
	t.Helper()
	tree, err := template.ParseTree("annotate", "annotate", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	options := analysis.WhitespaceOptions{AuthorCSSKnown: css != ""}
	if css != "" {
		options.Sheets = []analysis.Stylesheet{{Filename: "annotate.css", Text: css}}
	}
	cl := classify(tree.Root, analysis.ComputeWhitespace([]*template.Tree{tree}, options))

	type edit struct {
		start, end int
		marker     string
	}
	var edits []edit
	runEdit := func(text *template.TextNode, g gap) {
		base := int(text.Start())
		edits = append(edits, edit{base + g.start, base + g.end, "⟨" + runMarkers[g.class] + "⟩"})
	}
	for key, g := range cl.gaps {
		items := listItems(key.list)
		if len(items) == 0 {
			continue
		}
		if g.text != nil {
			runEdit(g.text, g)
			continue
		}
		marker := strings.ToLower(runMarkers[g.class])
		if g.class == runPreserved || g.class == runTrimmed {
			continue
		}
		var at int
		if key.index < len(items) {
			at = nodeStart(src, items[key.index])
		} else {
			at = nodeEnd(src, items[len(items)-1])
		}
		edits = append(edits, edit{at, at, "⟨" + marker + "⟩"})
	}
	for text, runs := range cl.runs {
		for _, g := range runs {
			runEdit(text, g)
		}
	}
	slices.SortFunc(edits, func(a, b edit) int { return cmp.Compare(b.start, a.start) })
	for _, e := range edits {
		src = src[:e.start] + e.marker + src[e.end:]
	}
	return src
}

var runMarkers = map[runClass]string{
	runPreserved:      "P",
	runTrimmed:        "T",
	runCollapse:       "C",
	runPreserveBreaks: "B",
}

// nodeStart and nodeEnd are n's source bounds including the delimiters of
// a template comment, whose node spans only the comment text.
func nodeStart(src string, n template.Node) int {
	if _, ok := n.(*template.CommentNode); ok {
		return strings.LastIndex(src[:n.Start()], "{{")
	}
	return int(n.Start())
}

func nodeEnd(src string, n template.Node) int {
	if _, ok := n.(*template.CommentNode); ok {
		return int(n.End()) + strings.Index(src[n.End():], "}}") + len("}}")
	}
	return int(n.End())
}

// TestUnsureParsedElsewhere checks which elements the insertion-mode model
// takes as built somewhere other than where the template tree puts them:
// it lists, in source order, the tags whose display unsureParsedElsewhere
// sets to unknown. A {{template}} call or {{block}} stands for what its
// body writes, resolved in the file or in the package's other templates
// (context), and a call to a template defined nowhere is taken to write
// what belongs where it stands.
func TestUnsureParsedElsewhere(t *testing.T) {
	const page = "<html>\n<head><title>T</title>%s</head>\n<body>\n<main><h1>x</h1></main>\n</body>\n</html>"
	for _, tt := range []struct {
		name    string
		src     string
		context []string
		want    []string
	}{
		{
			name: "call in head defined elsewhere",
			src:  strings.ReplaceAll(page, "%s", `{{template "head"}}`),
		},
		{
			name: "block in head writing head tags",
			src:  strings.ReplaceAll(page, "%s", `{{block "head" .}}<meta charset="utf-8">{{end}}`),
		},
		{
			name: "call in head resolving to head tags",
			src:  `{{define "h"}}<meta charset="utf-8">{{end}}` + strings.ReplaceAll(page, "%s", `{{template "h"}}`),
		},
		{
			name: "call in head resolving to text",
			src:  `{{define "h"}}x{{end}}<html><head>{{template "h"}}</head><body> <span>a</span></body></html>`,
			want: []string{"body"},
		},
		{
			name: "block in head writing an action",
			src:  "<html><head>{{block \"h\" .}}{{.X}}{{end}}</head>\n<body>\n <span>a</span></body></html>",
			want: []string{"body"},
		},
		{
			name: "call in a table section resolving to rows",
			src:  "{{define \"r\"}}<tr><td>{{.}}</td></tr>{{end}}<table>\n<tbody>\n{{range .Items}}\n{{template \"r\" .}}\n{{end}}\n</tbody>\n</table>",
			want: []string{"tr", "td"}, // the define body alone starts a document
		},
		{
			name: "call in a table section resolving to text",
			src:  "{{define \"r\"}}x{{end}}<table>\n<tbody>\n{{range .Items}}\n{{template \"r\" .}}\n{{end}}\n</tbody>\n</table>",
			want: []string{"table", "tbody"},
		},
		{
			name: "call in a table section defined elsewhere",
			src:  "<div><table>\n<tbody>\n{{range .Items}}\n{{template \"r\" .}}\n{{end}}\n</tbody>\n</table></div>",
		},
		{
			name: "call in a table section defined elsewhere, table parent not in the file",
			src:  "<table>\n<tbody>\n{{template \"r\" .}}\n</tbody>\n</table>",
			want: []string{"table", "tbody"},
		},
		{
			name: "call in a table section defined elsewhere, table parent preserves",
			src:  "<style>div{white-space:pre}</style><div><table>\n<tbody>\n{{template \"r\" .}}\n</tbody>\n</table></div>",
			want: []string{"table", "tbody"},
		},
		{
			name: "call in head defined elsewhere, body preserves",
			src:  "<html><head><style>body{white-space:pre-wrap}</style>{{template \"head\"}}</head><body> <span>a</span></body></html>",
			want: []string{"body"},
		},
		{
			name: "call in head defined elsewhere, no body in the file",
			src:  "<head>{{template \"head\"}}</head>",
		},
		{
			name:    "block in head overridden by text in the package",
			src:     `<html><head>{{block "extra" .}}{{end}}</head><body> <span>a</span></body></html>`,
			context: []string{`{{define "extra"}}t{{end}}`},
			want:    []string{"body"},
		},
		{
			name:    "block in a table section overridden by text in the package",
			src:     `<span>a</span><table><tbody>{{block "r" .}}<tr><td>x</td></tr>{{end}}</tbody></table>`,
			context: []string{`{{define "r"}}t{{end}}`},
			want:    []string{"table", "tbody", "tr", "td"}, // tr, td: the block body alone starts a document
		},
		{
			name:    "call in a table section with a name defined twice",
			src:     "<div><table>\n<tbody>\n{{template \"r\" .}}\n</tbody>\n</table></div>",
			context: []string{`{{define "r"}}<tr><td>{{.}}</td></tr>{{end}}`, `{{define "r"}}t{{end}}`},
			want:    []string{"table", "tbody"},
		},
		{
			name:    "empty define does not shadow a real one",
			src:     `<html><head>{{template "x"}}</head><body> <span>a</span></body></html>`,
			context: []string{`{{define "x"}}{{end}}`, `{{define "x"}}t{{end}}`},
			want:    []string{"body"},
		},
		{
			name: "mutually recursive calls in head",
			src:  `{{define "a"}}{{template "b"}}{{end}}{{define "b"}}{{template "a"}}{{end}}<html><head>{{template "a"}}</head><body> <span>a</span></body></html>`,
			want: []string{"body"},
		},
		{
			name: "recursive call in head",
			src:  `{{define "h"}}{{template "h"}}{{end}}<html><head>{{template "h"}}</head><body> <span>a</span></body></html>`,
			want: []string{"body"},
		},
		{
			name: "action in head",
			src:  `<html><head>{{.X}}</head><body> <span>a</span></body></html>`,
			want: []string{"body"},
		},
		{
			name:    "call in head resolving to text in the package",
			src:     `<html><head>{{template "h"}}</head><body> <span>a</span></body></html>`,
			context: []string{`{{define "h"}}x{{end}}`},
			want:    []string{"body"},
		},
		{
			name:    "call in head resolving to head tags in the package",
			src:     `<html><head>{{template "h"}}</head><body> <span>a</span></body></html>`,
			context: []string{`{{define "h"}}<meta charset="utf-8">{{end}}`},
		},
		{
			name:    "call in a table section resolving to text in the package",
			src:     "<table>\n<tbody>\n{{template \"r\" .}}\n</tbody>\n</table>",
			context: []string{`{{define "r"}}x{{end}}`},
			want:    []string{"table", "tbody"},
		},
		{
			name:    "call in a table section resolving to rows in the package",
			src:     "<table>\n<tbody>\n{{template \"r\" .}}\n</tbody>\n</table>",
			context: []string{`{{define "r"}}<tr><td>{{.}}</td></tr>{{end}}`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parse := func(name, src string) *template.Tree {
				tree, err := template.ParseTree(name, name, src, nil, lex.Options{
					EmitComment: true,
					ParseHTML:   true,
				})
				if err != nil {
					t.Fatalf("parse %q: %v", src, err)
				}
				return tree
			}
			tree := parse("input.gohtml", tt.src)
			var context []*template.Tree
			for i, src := range tt.context {
				context = append(context, parse(fmt.Sprintf("context%d.gohtml", i), src))
			}
			styles := analysis.ComputeWhitespace([]*template.Tree{tree}, analysis.WhitespaceOptions{Context: context, AuthorCSSKnown: true})
			var unknown []*template.HTMLElementNode
			for _, m := range parsedElsewhere(tree.Root, styles, context) {
				if !slices.Contains(unknown, m.el) {
					unknown = append(unknown, m.el)
				}
			}
			slices.SortFunc(unknown, func(a, b *template.HTMLElementNode) int { return cmp.Compare(a.Start(), b.Start()) })
			var got []string
			for _, el := range unknown {
				got = append(got, strings.ToLower(el.TagName))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("parsedElsewhere(%q) marks %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

// TestWhiteSpaceOnly checks that only white-space decides whether the
// formatter may reindent inside an element (v2 spec §2): display does
// not, a class no white-space-declaring rule can match is harmless, and
// custom elements, SVG text content and MathML token elements are left
// as written.
func TestWhiteSpaceOnly(t *testing.T) {
	for _, tt := range []struct {
		name    string
		src     string
		css     string
		known   bool // the package was loaded: the visible CSS is all of it
		missing bool // a stylesheet could not be loaded
		want    string
	}{
		{
			name:  "a class matched only by display reindents",
			src:   "<style>.card{display:block}</style><div class=\"card\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<style>.card{display:block}</style><div class=\"card\">\n\t<p>a</p>\n</div>",
		},
		{
			name:  "a class matched by white-space preserve is exact",
			src:   "<style>.bubble{white-space:pre-wrap}</style><div class=\"bubble\">\n x</div>",
			known: true,
			want:  "<style>.bubble{white-space:pre-wrap}</style><div class=\"bubble\">\n x</div>",
		},
		// all: unset makes white-space inherit, which is known; a value
		// of all ComputeWhitespace cannot read, or one that may apply, is
		// unsure.
		{
			name:  "all unset inherits",
			src:   "<style>.x{all:unset}</style><div class=\"x\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<style>.x{all:unset}</style><div class=\"x\">\n\t<p>a</p>\n</div>",
		},
		{
			name:  "all revert is unsure",
			src:   "<style>.x{all:revert}</style><div class=\"x\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<style>.x{all:revert}</style><div class=\"x\">\n<p>a</p>\n</div>",
		},
		{
			name:  "all that may apply is unsure",
			src:   "<style>@media print{.x{all:revert}}</style><div class=\"x\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<style>@media print{.x{all:revert}}</style><div class=\"x\">\n<p>a</p>\n</div>",
		},
		{
			name:  "a class no rule names reindents",
			src:   "<div class=\"unknown\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<div class=\"unknown\">\n\t<p>a</p>\n</div>",
		},
		{
			name:    "a class with a missing sheet is exact",
			src:     "<div class=\"unknown\">\n<p>a</p>\n</div>",
			known:   true,
			missing: true,
			want:    "<div class=\"unknown\">\n<p>a</p>\n</div>",
		},
		{
			name:  "a dynamic class with no white-space rule reindents",
			src:   "<style>.c{display:flex}</style><div class=\"{{.C}}\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<style>.c{display:flex}</style><div class=\"{{.C}}\">\n\t<p>a</p>\n</div>",
		},
		{
			name:  "a dynamic class with a type-selector white-space rule reindents",
			src:   "<div class=\"{{.C}}\">\n<p>a</p>\n</div>",
			css:   "code{white-space:pre}",
			known: true,
			want:  "<div class=\"{{.C}}\">\n\t<p>a</p>\n</div>",
		},
		{
			name:  "a dynamic class with a class white-space rule is exact",
			src:   "<div class=\"{{.C}}\">\n<p>a</p>\n</div>",
			css:   ".code{white-space:pre}",
			known: true,
			want:  "<div class=\"{{.C}}\">\n<p>a</p>\n</div>",
		},
		{
			name:  "a dynamic id with an id white-space rule is exact",
			src:   "<div id=\"{{.ID}}\">\n<p>a</p>\n</div>",
			css:   "#log{white-space:pre}",
			known: true,
			want:  "<div id=\"{{.ID}}\">\n<p>a</p>\n</div>",
		},
		{
			name:  "a conditional class with a class white-space rule is exact",
			src:   "<div {{if .X}}class=\"code\"{{end}}>\n<p>a</p>\n</div>",
			css:   ".code{white-space:pre}",
			known: true,
			want:  "<div {{if .X}}class=\"code\"{{end}}>\n<p>a</p>\n</div>",
		},
		{
			name:  "a conditional class with no white-space rule reindents",
			src:   "<div {{if .X}}class=\"card\"{{end}}>\n<p>a</p>\n</div>",
			css:   ".card{display:none}",
			known: true,
			want:  "<div {{if .X}}class=\"card\"{{end}}>\n\t<p>a</p>\n</div>",
		},
		{
			name:  "a style attribute with an action is exact",
			src:   "<div style=\"{{.S}}\">\n<p>a</p>\n</div>",
			known: true,
			want:  "<div style=\"{{.S}}\">\n<p>a</p>\n</div>",
		},
		{
			name: "a display none element reindents inside",
			src:  "<div hidden>\n<p>a</p>\n</div>",
			want: "<div hidden>\n\t<p>a</p>\n</div>",
		},
		{
			name: "a conditional hidden reindents inside",
			src:  "<div {{if .X}}hidden{{end}}>\n<p>a</p>\n</div>",
			want: "<div {{if .X}}hidden{{end}}>\n\t<p>a</p>\n</div>",
		},
		{
			name: "head reindents",
			src:  "<head>\n<meta>\n</head>",
			want: "<head>\n\t<meta>\n</head>",
		},
		{
			name: "a custom element is exact",
			src:  "<my-chip>a\n b</my-chip>",
			want: "<my-chip>a\n b</my-chip>",
		},
		{
			name: "a customized built-in element is exact",
			src:  "<div>\n<p is=\"my-p\">a\n b</p>\n</div>",
			want: "<div>\n<p is=\"my-p\">a\n b</p>\n</div>",
		},
		{
			name: "SVG text is exact",
			src:  "<svg><text>a\n b</text></svg>",
			want: "<svg><text>a\n b</text></svg>",
		},
		{
			name: "SVG tspan and textPath are exact",
			src:  "<svg>\n<text><tspan>a\n b</tspan><textPath>c\n d</textPath></text>\n</svg>",
			want: "<svg>\n<text><tspan>a\n b</tspan><textPath>c\n d</textPath></text>\n</svg>",
		},
		{
			name: "MathML token elements are exact",
			src:  "<math>\n<mi>x\n y</mi>\n<mo>+\n</mo>\n<mtext>a\n b</mtext>\n</math>",
			want: "<math>\n<mi>x\n y</mi>\n<mo>+\n</mo>\n<mtext>a\n b</mtext>\n</math>",
		},
		{
			name: "SVG structure reindents around opaque text",
			src:  "<svg>\n<g>\n<rect/>\n<text>a\n b</text>\n</g>\n</svg>",
			want: "<svg>\n\t<g>\n\t\t<rect/>\n<text>a\n b</text>\n\t</g>\n</svg>",
		},
		{
			name: "SVG text directly under an SVG element is exact",
			src:  "<svg>\n<g>a\n b</g>\n</svg>",
			want: "<svg>\n<g>a\n b</g>\n</svg>",
		},
		{
			name: "foreignObject content is HTML",
			src:  "<svg>\n<foreignObject>\n<div>\n<p>a</p>\n</div>\n</foreignObject>\n</svg>",
			want: "<svg>\n\t<foreignObject>\n\t\t<div>\n\t\t\t<p>a</p>\n\t\t</div>\n\t</foreignObject>\n</svg>",
		},
		// Ownership follows the browser's tree: where the candidate
		// owners of a run differ in white-space, the run is as written.
		{
			name: "a reopened formatting element that collapses",
			src:  "<div><p><b>x</p>\n<p>\ny</p></div>",
			want: "<div><p><b>x</p>\n\t<p>\n\t\ty</p></div>",
		},
		{
			name:  "a reopened formatting element that preserves",
			src:   "<div><p><b>x</p>\n<p>\ny</p></div>",
			css:   "b{white-space:pre}",
			known: true,
			want:  "<div><p><b>x</p>\n<p>\ny</p></div>",
		},
		{
			name: "foster-parented into a pre-line parent",
			src:  "<div style=\"white-space:pre-line\">\n<table style=\"white-space:normal\">x\n y</table>\n</div>",
			want: "<div style=\"white-space:pre-line\">\n<table style=\"white-space:normal\">x\n y</table>\n</div>",
		},
		{
			name: "a closed p's content lands in a pre-line parent",
			src:  "<div style=\"white-space:pre-line\"><p style=\"white-space:normal\">a<div>\nb</div>\nc\n  d</p></div>",
			want: "<div style=\"white-space:pre-line\"><p style=\"white-space:normal\">a<div>\nb</div>\nc\n  d</p></div>",
		},
		{
			name: "a closed p's content lands in a parent that collapses",
			src:  "<div><p>a<div>\nb</div>\nc\n  d</p></div>",
			want: "<div><p>a<div>\n\t\t\tb</div>\n\t\tc\n\t\td</p></div>",
		},
		{
			name: "a custom element's parent still reindents",
			src:  "<div>\n<p>a</p>\n<my-chip>a\n b</my-chip>\n</div>",
			want: "<div>\n\t<p>a</p>\n<my-chip>a\n b</my-chip>\n</div>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := analysis.WhitespaceOptions{AuthorCSSKnown: tt.known, MissingSheets: tt.missing}
			if tt.css != "" {
				options.Sheets = []analysis.Stylesheet{{Filename: "site.css", Text: tt.css}}
			}
			got, err := Source("input.gohtml", tt.src, Styles(options))
			if err != nil {
				t.Fatalf("Source(%q): %v", tt.src, err)
			}
			if got != tt.want {
				t.Errorf("Source(%q) =\n%q\nwant\n%q", tt.src, got, tt.want)
			}
		})
	}
}

// TestMovedElementSelectors checks that an element the browser builds
// somewhere other than where the template tree puts it is as written
// when a rule declaring white-space has a selector that depends on the
// tree (a combinator or a structural, relational or negation
// pseudo-class): the browser matches it in its own tree. The oracle
// renders both and compares them in the browser's tree. With only
// simple selectors the moved element still reindents (changed).
func TestMovedElementSelectors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		src     string
		changed bool
	}{
		{name: "misnested child", src: "<style>section > div{white-space:pre}</style><section><p><div>\n  a</div></p></section>"},
		{name: "foster-parented", src: "<style>div > p{white-space:pre}</style><div><table><p>\n  a</p></table></div>"},
		{name: "dropped td", src: "<style>div > p{white-space:pre}</style><div><td><p>\n  a</p></td></div>"},
		{name: "negation", src: "<style>div:not(p div){white-space:pre}</style><p><div>\n  a</div></p>"},
		{name: "sibling", src: "<style>p + div{white-space:pre}</style><section><p>x<div>\n  a</div></p></section>"},
		{name: "structural", src: "<style>div:first-child{white-space:pre}</style><section><p><div>\n  a</div></p></section>"},
		{name: "reopened", src: "<style>div > b{white-space:pre}</style><div><p><b>x</p>\n<p>\n  y</p></div>"},
		{name: "misnested child, simple selector", src: "<style>.k{white-space:pre}</style><section><p><div>\n  a</div></p></section>", changed: true},
		{name: "foster-parented, simple selector", src: "<style>.k{white-space:pre}</style><div><table><p>\n  a</p></table></div>", changed: true},
		{name: "dropped td, simple selector", src: "<style>.k{white-space:pre}</style><div><td><p>\n  a</p></td></div>", changed: true},
		{name: "reopened, simple selector", src: "<style>.k{white-space:pre}</style><div><p><b>x</p>\n<p>\n  y</p></div>", changed: true},
		{name: "a combinator elsewhere leaves unmoved elements alone", src: "<style>main > p{white-space:pre}</style><div>\n<p>a</p>\n</div>", changed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Source("input.gohtml", tt.src, Styles(analysis.WhitespaceOptions{AuthorCSSKnown: true}))
			if err != nil {
				t.Fatalf("Source(%q): %v", tt.src, err)
			}
			diff, err := rendering.Diff(tt.src, got, rendering.Body, nil)
			if err != nil {
				t.Fatalf("rendering.Diff: %v", err)
			}
			if diff != "" {
				t.Errorf("Source(%q) = %q changes rendering:\n%s", tt.src, got, diff)
			}
			if changed := got != tt.src; changed != tt.changed {
				t.Errorf("Source(%q) = %q, changed %v, want %v", tt.src, got, changed, tt.changed)
			}
		})
	}
}

// TestShadowHosts checks that an element whose light-DOM text may be
// slotted under CSS the formatter cannot see is written as it is.
func TestShadowHosts(t *testing.T) {
	for _, tt := range []struct {
		name, src, want string
	}{
		{
			name: "conditional is on a div",
			src:  "<div {{if .X}}is=\"x-y\"{{end}}>\n  a</div>",
			want: "<div {{if .X}}is=\"x-y\"{{end}}>\n  a</div>",
		},
		{
			name: "conditional is on a p",
			src:  "<p {{if .X}}is=\"x-y\"{{end}}>a\n b</p>",
			want: "<p {{if .X}}is=\"x-y\"{{end}}>a\n b</p>",
		},
		{
			name: "declarative shadow root",
			src:  "<div><template shadowrootmode=\"open\"><style>slot{white-space:pre}</style><slot></slot></template>\n  a\n  b</div>",
			want: "<div><template shadowrootmode=\"open\"><style>slot{white-space:pre}</style><slot></slot></template>\n  a\n  b</div>",
		},
		{
			name: "legacy declarative shadow root",
			src:  "<div><template shadowroot=\"open\"><slot></slot></template>\n  a\n  b</div>",
			want: "<div><template shadowroot=\"open\"><slot></slot></template>\n  a\n  b</div>",
		},
		{
			name: "template contents reindent",
			src:  "<div><template>\n<p>a</p>\n</template>\n  a\n</div>",
			want: "<div><template>\n\t\t<p>a</p>\n\t</template>\n\ta\n</div>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Source("input.gohtml", tt.src)
			if err != nil {
				t.Fatalf("Source(%q): %v", tt.src, err)
			}
			if got != tt.want {
				t.Errorf("Source(%q) =\n%q\nwant\n%q", tt.src, got, tt.want)
			}
		})
	}
}
