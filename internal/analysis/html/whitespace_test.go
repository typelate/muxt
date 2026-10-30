package analysis

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
	"github.com/typelate/muxt/internal/template/selectors"
)

func TestComputeWhitespace(t *testing.T) {
	for _, test := range []struct {
		source   string
		sheet    string // a stylesheet file, when not empty
		context  string // a context template, when not empty
		imported string // a second stylesheet file, site.css, when not empty
		known    bool   // AuthorCSSKnown
		selector string // picks the element to check
		want     Whitespace
	}{
		{source: `<div></div>`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<span></span>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<my-widget></my-widget>`, selector: `my-widget`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<label></label>`, selector: `label`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<img src=a>`, selector: `img`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
		{source: `<button></button>`, selector: `button`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
		{source: `<input>`, selector: `input`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
		{source: `<br>`, selector: `br`, want: Whitespace{DisplayNewline, WhiteSpaceCollapse}},
		{source: `<table><tr><td></td></tr></table>`, selector: `td`, want: Whitespace{DisplayTableInternal, WhiteSpaceCollapse}},
		{source: `<table><tr><td></td></tr></table>`, selector: `tr`, want: Whitespace{DisplayTableInternal, WhiteSpaceCollapse}},
		{source: `<table><caption></caption></table>`, selector: `caption`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div hidden></div>`, selector: `div`, want: Whitespace{DisplayNone, WhiteSpaceCollapse}},
		{source: `<slot></slot>`, selector: `slot`, want: Whitespace{DisplayContents, WhiteSpaceCollapse}},
		{source: `<pre></pre>`, selector: `pre`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<pre wrap></pre>`, selector: `pre`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<textarea></textarea>`, selector: `textarea`, want: Whitespace{DisplayAtomic, WhiteSpacePreserve}},
		{source: `<script></script>`, selector: `script`, want: Whitespace{DisplayNone, WhiteSpacePreserve}},
		{source: `<table><tr><td nowrap></td></tr></table>`, selector: `td`, want: Whitespace{DisplayTableInternal, WhiteSpaceCollapse}},
		{source: `<h1></h1>`, selector: `h1`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<span style="display:block"></span>`, selector: `span`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<ul style="display:flex"></ul>`, selector: `ul`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<p style="white-space:pre-line"></p>`, selector: `p`, want: Whitespace{DisplayBlock, WhiteSpacePreserveBreaks}},
		{source: `<p style="white-space-collapse:preserve"></p>`, selector: `p`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<p style="color: {{.C}}"></p>`, selector: `p`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<p style="{{.S}}"></p>`, selector: `p`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<style>.c{white-space:pre}</style><div class="c"></div>`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<style>.c{white-space:pre}</style><div class="c x"></div>`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<div id="main"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="{{.C}}"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, sheet: `@media print{.c{white-space:pre}}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, sheet: `.c{display:inline!important}.c{display:block}`, selector: `div`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, sheet: `div.c{display:inline}.c{display:block}`, selector: `div`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div {{if .H}}hidden{{end}}></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<pre><span></span></pre>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpacePreserve}},
		{source: `<div style="white-space:pre"><span style="white-space:normal"></span></div>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpacePreserve}},
		{source: `{{define "x"}}<span></span>{{end}}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, context: `<style>.c{display:block}</style>`, selector: `span`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},

		// Beyond the brief's table: conditional and dynamic author CSS.
		{source: `<style>{{if .X}}pre{white-space:normal}{{end}}</style><pre></pre>`, selector: `pre`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<style>{{.CSS}}</style><p></p>`, selector: `p`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<style>.c{color:{{.C}}}</style><div class="c"></div>`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<style>.c{display:{{.D}}}</style><div class="c"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.item-{{.N}}{display:inline}</style><div></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.c{display:inline}</style><span class="c"></span>`, sheet: `.c{display:block}`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.a .c{display:block}</style><div class="a"><span class="c"></span></div>`, selector: `span`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div {{if .X}}class="c"{{end}}></div>`, sheet: `.c{}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, sheet: `@layer base{.c{display:inline}}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, sheet: `.c{&>p{display:inline}}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="c"><p></p></div>`, sheet: `.c{&>p{display:inline}}`, selector: `p`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, sheet: `.c::before{display:block}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, sheet: `.c{@media print{display:block}}`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, sheet: `.c:hover{white-space:pre}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceUnknown}},
		{source: `<span {{if .X}}id="a"{{end}}></span>`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},

		// Review fixes (2026-10-30).
		{source: `<div class="c"></div>`, known: true, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div style="color:red {{if .X}}; display:none; color:blue{{end}}"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<span class="c"></span>`, sheet: `.c{display-outside:block}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span style="display-outside:block"></span>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div class="code"></div>`, sheet: `@import url(site.css);`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="code"></div>`, sheet: `@import url(site.css);`, imported: `p{color:red}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="code"></div>`, sheet: `@import "site.css";`, imported: `p{color:red}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<style>@import url(other.css);</style><div class="code"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},

		{source: `<noscript></noscript>`, selector: `noscript`, want: Whitespace{DisplayUnknown, WhiteSpacePreserve}},
		{source: `<p style="white-space:inherit"></p>`, selector: `p`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<img style="display:inline">`, selector: `img`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
		{source: `<span style="display:inline flow-root"></span>`, selector: `span`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
	} {
		t.Run(test.source, func(t *testing.T) {
			trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", test.source)}
			var context []*template.Tree
			if test.context != "" {
				context = append(context, parseWhitespaceTree(t, "context.gohtml", test.context))
			}
			var sheets []Stylesheet
			if test.sheet != "" {
				sheets = append(sheets, Stylesheet{Filename: "static/sheet.css", Text: test.sheet})
			}
			if test.imported != "" {
				sheets = append(sheets, Stylesheet{Filename: "static/site.css", Text: test.imported})
			}
			el := queryElement(t, trees, test.selector)
			got, ok := ComputeWhitespace(trees, WhitespaceOptions{Context: context, Sheets: sheets, AuthorCSSKnown: test.known})[el]
			if !ok || got != test.want {
				t.Errorf("ComputeWhitespace(%q)[%s] = %+v, want %+v", test.source, test.selector, got, test.want)
			}
		})
	}
}

func TestComputeWhitespaceClassifiesEveryElement(t *testing.T) {
	trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", `<div><p>{{if .X}}<b>x</b>{{else}}<i>y</i>{{end}}</p>{{range .}}<li></li>{{end}}</div>{{define "d"}}<em></em>{{end}}`)}
	got := ComputeWhitespace(trees, WhitespaceOptions{})
	for _, name := range []string{"div", "p", "b", "i", "li", "em"} {
		el := queryElement(t, trees, name)
		if _, ok := got[el]; !ok {
			t.Errorf("ComputeWhitespace has no entry for <%s>", name)
		}
	}
	if len(got) != 6 {
		t.Errorf("len(ComputeWhitespace(...)) = %d, want 6", len(got))
	}
}

func parseWhitespaceTree(t *testing.T, name, source string) *template.Tree {
	t.Helper()
	tree, err := template.ParseTree(name, name, source, nil, lex.NewOptions())
	if err != nil {
		t.Fatalf("ParseTree(%q) error: %v", source, err)
	}
	return tree
}

func queryElement(t *testing.T, trees []*template.Tree, selector string) *template.HTMLElementNode {
	t.Helper()
	sel := selectors.MustCompile(selector)
	for _, root := range templateRoots(trees) {
		if el, _ := sel.QuerySelector(root); el != nil {
			return el
		}
	}
	t.Fatalf("no element matches %q", selector)
	return nil
}
