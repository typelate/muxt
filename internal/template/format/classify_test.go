package format

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		src, css string
		want     string
	}{
		{
			src:  "<div><span>a</span><span>b</span>\n</div>",
			want: "⟨r⟩<div>⟨r⟩<span>⟨r⟩a⟨c⟩</span>⟨c⟩<span>⟨c⟩b⟨r⟩</span>⟨R⟩</div>⟨r⟩",
		},
		{
			src:  "<div>\n<b>Hi</b>{{.Name}}\n</div>",
			want: "⟨r⟩<div>⟨R⟩<b>⟨r⟩Hi⟨c⟩</b>⟨c⟩{{.Name}}⟨R⟩</div>⟨r⟩",
		},
		{
			src:  "<div>\n x{{if .Show}}<b>y</b>{{end}}\n</div>",
			want: "⟨r⟩<div>⟨R⟩x⟨c⟩{{if .Show}}⟨c⟩<b>⟨c⟩y⟨r⟩</b>⟨r⟩{{end}}⟨R⟩</div>⟨r⟩",
		},
		{
			src:  "<ul>{{range .}}<li>x</li>{{end}}</ul>",
			want: "⟨r⟩<ul>⟨r⟩{{range .}}⟨r⟩<li>⟨r⟩x⟨r⟩</li>⟨r⟩{{end}}⟨r⟩</ul>⟨r⟩",
		},
		{
			src:  "<p>a {{- .X}}</p>",
			want: "⟨r⟩<p>⟨r⟩a⟨T⟩{{- .X}}⟨r⟩</p>⟨r⟩",
		},
		{
			src:  "<div>a<div hidden></div>b\n</div>",
			want: "⟨r⟩<div>⟨r⟩a⟨c⟩<div hidden></div>⟨c⟩b⟨R⟩</div>⟨r⟩",
		},
		{
			src:  "<p>a <span class=\"x\">\n b </span> c</p>",
			want: "⟨r⟩<p>⟨r⟩a⟨C⟩<span class=\"x\">\n b </span>⟨C⟩c⟨r⟩</p>⟨r⟩",
		},
		{
			src:  "{{define \"x\"}}\n<span>a</span>\n{{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩<span>⟨c⟩a⟨c⟩</span>⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "{{define \"x\"}}\n<div>a</div>\n{{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨R⟩<div>⟨r⟩a⟨r⟩</div>⟨R⟩{{end}}⟨c⟩",
		},
		{
			src:  "<pre>\n  a  </pre>",
			want: "⟨r⟩<pre>\n  a  </pre>⟨r⟩",
		},
		{
			src:  "<p style=\"white-space:pre-line\">a \n b</p>",
			want: "⟨r⟩<p style=\"white-space:pre-line\">⟨b⟩a⟨B⟩b⟨b⟩</p>⟨r⟩",
		},
		{
			src:  "<div>a{{/* c */}}b</div>",
			want: "⟨r⟩<div>⟨r⟩a⟨c⟩{{/* c */}}⟨c⟩b⟨r⟩</div>⟨r⟩",
		},
		{
			src:  "Dear {{.Name}},\n\nThanks",
			want: "Dear⟨P⟩{{.Name}},⟨P⟩Thanks",
		},
		// A break ends the range next to what follows it.
		{
			src:  "<p>{{range .}}<b>s</b>{{break}}{{end}} y</p>",
			want: "⟨r⟩<p>⟨r⟩{{range .}}⟨r⟩<b>⟨r⟩s⟨c⟩</b>⟨c⟩{{break}}⟨r⟩{{end}}⟨C⟩y⟨r⟩</p>⟨r⟩",
		},
		// A continue meets the next iteration's start.
		{
			src:  "<p>{{range .}}<b>s</b>{{continue}}{{end}}</p>",
			want: "⟨r⟩<p>⟨r⟩{{range .}}⟨c⟩<b>⟨c⟩s⟨c⟩</b>⟨c⟩{{continue}}⟨r⟩{{end}}⟨r⟩</p>⟨r⟩",
		},
		// Iteration n ends next to iteration n+1.
		{
			src:  "<p>{{range .}}<b>x</b>{{end}}</p>",
			want: "⟨r⟩<p>⟨r⟩{{range .}}⟨c⟩<b>⟨c⟩x⟨c⟩</b>⟨c⟩{{end}}⟨r⟩</p>⟨r⟩",
		},
		// br is an edge; an atomic box is opaque outside and has edges
		// inside; display: contents children join the parent's flow.
		{
			src:  "<div>a<br>b<span style=\"display:inline-block\">c</span><slot>d</slot></div>",
			want: "⟨r⟩<div>⟨r⟩a⟨r⟩<br>⟨r⟩b⟨c⟩<span style=\"display:inline-block\">⟨r⟩c⟨r⟩</span>⟨c⟩<slot>⟨c⟩d⟨r⟩</slot>⟨r⟩</div>⟨r⟩",
		},
		// The inside of a display: none element has no certain edges.
		{
			src:  "<div><div hidden> x </div></div>",
			want: "⟨r⟩<div>⟨r⟩<div hidden>⟨C⟩x⟨C⟩</div>⟨r⟩</div>⟨r⟩",
		},
		{
			src:  "<p>{{if .X -}} a {{- end}}</p>",
			want: "⟨r⟩<p>⟨r⟩{{if .X -}}⟨T⟩a⟨T⟩{{- end}}⟨r⟩</p>⟨r⟩",
		},
		// A define body's edges are reflowable when every first (last)
		// item is block-level whatever the branch.
		{
			src:  "{{define \"x\"}} {{if .A}}<div>a</div>{{else}}<p>b</p>{{end}} {{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨R⟩{{if .A}}⟨r⟩<div>⟨r⟩a⟨r⟩</div>⟨r⟩{{else}}⟨r⟩<p>⟨r⟩b⟨r⟩</p>⟨r⟩{{end}}⟨R⟩{{end}}⟨c⟩",
		},
		{
			src:  "{{define \"x\"}} {{if .A}}<div>a</div>{{end}} {{end}}",
			want: "⟨c⟩{{define \"x\"}}⟨C⟩{{if .A}}⟨r⟩<div>⟨r⟩a⟨r⟩</div>⟨r⟩{{end}}⟨C⟩{{end}}⟨c⟩",
		},
		{
			src:  "<table><tr><td>a</td> <td>b</td></tr></table>",
			want: "⟨r⟩<table>⟨r⟩<tr>⟨r⟩<td>⟨r⟩a⟨r⟩</td>⟨R⟩<td>⟨r⟩b⟨r⟩</td>⟨r⟩</tr>⟨r⟩</table>⟨r⟩",
		},
		{
			src:  "<p>a <em class=\"k\"> b </em> c</p>",
			css:  ".k{white-space:pre}",
			want: "⟨r⟩<p>⟨r⟩a⟨C⟩<em class=\"k\"> b </em>⟨C⟩c⟨r⟩</p>⟨r⟩",
		},
		{
			src:  "<p>a <em class=\"k\"> b </em> c</p>",
			css:  ".k{display:block}",
			want: "⟨r⟩<p>⟨r⟩a⟨R⟩<em class=\"k\">⟨R⟩b⟨R⟩</em>⟨R⟩c⟨r⟩</p>⟨r⟩",
		},
		{
			src:  "<p> a  b </p>",
			want: "⟨r⟩<p>⟨R⟩a⟨C⟩b⟨R⟩</p>⟨r⟩",
		},
		{
			src:  "<p><b>x</b> y <b>z</b></p>",
			want: "⟨r⟩<p>⟨r⟩<b>⟨r⟩x⟨c⟩</b>⟨C⟩y⟨C⟩<b>⟨c⟩z⟨r⟩</b>⟨r⟩</p>⟨r⟩",
		},
	} {
		if got := annotate(t, tt.src, tt.css); got != tt.want {
			t.Errorf("annotate(%q) =\n%s\nwant\n%s", tt.src, got, tt.want)
		}
	}
}

// annotate returns src with each non-empty whitespace run replaced by a
// marker of its class (⟨R⟩ reflowable, ⟨C⟩ collapse-only, ⟨P⟩ preserved,
// ⟨T⟩ trimmed, ⟨B⟩ preserve-breaks) and each empty gap marked ⟨r⟩, ⟨c⟩
// or ⟨b⟩. Runs inside opaque subtrees and gaps of lists with no items get
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
	runReflowable:     "R",
	runCollapseOnly:   "C",
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
