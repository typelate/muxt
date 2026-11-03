package analysis

import (
	"fmt"
	"strings"
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
		missing  bool   // MissingSheets
		selector string // picks the element to check
		want     Whitespace
	}{
		// Rows that test how author CSS cascades onto a class or id set
		// known: a <style> element or sheet alone no longer makes the
		// author CSS known (Task 21), so without it those elements are
		// unknown whatever the rules say.
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
		{source: `<style>.c{white-space:pre}</style><div class="c"></div>`, known: true, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<style>.c{white-space:pre}</style><div class="c x"></div>`, known: true, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpacePreserve}},
		{source: `<div id="main"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="{{.C}}"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, known: true, sheet: `@media print{.c{white-space:pre}}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, known: true, sheet: `.c{display:inline!important}.c{display:block}`, selector: `div`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, known: true, sheet: `div.c{display:inline}.c{display:block}`, selector: `div`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		// A conditional hidden changes only display.
		{source: `<div {{if .H}}hidden{{end}}></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		// A dynamic or conditional class or id is left to the selector
		// matcher: it makes white-space unknown only through a rule that
		// declares white-space and has a selector it may match.
		{source: `<div class="{{.C}}"></div>`, known: true, sheet: `.c{display:flex}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="{{.C}}"></div>`, known: true, sheet: `code{white-space:pre}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="{{.C}}"></div>`, known: true, sheet: `.code{white-space:pre}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<div class="a {{.C}}"></div>`, known: true, sheet: `[class~=code]{white-space:pre-line}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<div id="{{.ID}}"></div>`, known: true, sheet: `#log{white-space:pre}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<div id="{{.ID}}"></div>`, known: true, sheet: `.log{white-space:pre}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="{{.C}}"><span></span></div>`, known: true, sheet: `.code span{white-space:pre}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceUnknown}},
		{source: `<div {{if .X}}class="c"{{end}}></div>`, known: true, sheet: `.c{display:none}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div {{if .X}}class="c"{{end}}></div>`, known: true, sheet: `.c{white-space:pre}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div {{if .X}}style="white-space:pre"{{end}}></div>`, known: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="{{.C}}"></div>`, known: true, missing: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		// all: initial and unset are known; any other value is unknown.
		{source: `<div class="x"></div>`, known: true, sheet: `.x{all:unset}`, selector: `div`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div class="x"></div>`, known: true, sheet: `.x{all:revert}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="x"></div>`, known: true, sheet: `@media print{.x{all:unset}}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<pre><span></span></pre>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpacePreserve}},
		{source: `<div style="white-space:pre"><span style="white-space:normal"></span></div>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpacePreserve}},
		{source: `{{define "x"}}<span></span>{{end}}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, known: true, context: `<style>.c{display:block}</style>`, selector: `span`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},

		// Beyond the brief's table: conditional and dynamic author CSS.
		{source: `<style>{{if .X}}pre{white-space:normal}{{end}}</style><pre></pre>`, selector: `pre`, want: Whitespace{DisplayBlock, WhiteSpaceUnknown}},
		{source: `<style>{{.CSS}}</style><p></p>`, selector: `p`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<style>.c{color:{{.C}}}</style><div class="c"></div>`, known: true, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<style>.c{display:{{.D}}}</style><div class="c"></div>`, known: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.item-{{.N}}{display:inline}</style><div></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.c{display:inline}</style><span class="c"></span>`, known: true, sheet: `.c{display:block}`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<style>.a .c{display:block}</style><div class="a"><span class="c"></span></div>`, known: true, selector: `span`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div {{if .X}}class="c"{{end}}></div>`, sheet: `.c{}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="c"></div>`, known: true, sheet: `@layer base{.c{display:inline}}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, known: true, sheet: `.c{&>p{display:inline}}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="c"><p></p></div>`, known: true, sheet: `.c{&>p{display:inline}}`, selector: `p`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, known: true, sheet: `.c::before{display:block}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, known: true, sheet: `.c{@media print{display:block}}`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<span class="c"></span>`, known: true, sheet: `.c:hover{white-space:pre}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceUnknown}},
		{source: `<span {{if .X}}id="a"{{end}}></span>`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},

		// Review fixes (2026-10-30).
		{source: `<div class="c"></div>`, known: true, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="c"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div style="color:red {{if .X}}; display:none; color:blue{{end}}"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<span class="c"></span>`, known: true, sheet: `.c{display-outside:block}`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<span style="display-outside:block"></span>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div class="code"></div>`, sheet: `@import url(site.css);`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="code"></div>`, known: true, sheet: `@import url(site.css);`, imported: `p{color:red}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="code"></div>`, known: true, sheet: `@import "site.css";`, imported: `p{color:red}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<style>@import url(other.css);</style><div class="code"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div class="code"></div>`, sheet: `@import "/site.css";`, imported: `p{color:red}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div></div>`, missing: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<style>p{color:red}</style><div class="code"></div>`, missing: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		// Task 21: a <style> element is not all the author CSS. Without
		// AuthorCSSKnown a class or id stays unknown, however many <style>
		// elements or sheets are visible.
		{source: `<style>p{color:red}</style><div class="code"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
		{source: `<div id="main"></div>`, sheet: `p{color:red}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},

		{source: `<noscript></noscript>`, selector: `noscript`, want: Whitespace{DisplayUnknown, WhiteSpacePreserve}},
		{source: `<p style="white-space:inherit"></p>`, selector: `p`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<img style="display:inline">`, selector: `img`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},
		{source: `<span style="display:inline flow-root"></span>`, selector: `span`, want: Whitespace{DisplayAtomic, WhiteSpaceCollapse}},

		// Out-of-flow boxes (spec amendment, 2026-10-30): floats and
		// absolutely positioned boxes are not in the inline flow.
		{source: `<span style="float:left"></span>`, selector: `span`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<span style="float:none"></span>`, selector: `span`, want: Whitespace{DisplayInline, WhiteSpaceCollapse}},
		{source: `<div style="position:absolute"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div style="position:fixed"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div style="position:relative"></div>`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div class="f"></div>`, known: true, sheet: `.f{float:right}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div class="f"></div>`, known: true, sheet: `.f{position:fixed}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div class="f"></div>`, known: true, sheet: `.f{position:absolute}div{position:static!important}`, selector: `div`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<style>.f{float:left}</style><div class="f"></div>`, known: true, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<div class="f"></div>`, known: true, sheet: `@media print{.f{float:left}}`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		// The UA sheet's floats and positioned boxes (Task 5 carry-forward).
		{source: `<dialog open></dialog>`, selector: `dialog`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<dialog></dialog>`, selector: `dialog`, want: Whitespace{DisplayNone, WhiteSpaceCollapse}},
		{source: `<div hidden style="float:left"></div>`, selector: `div`, want: Whitespace{DisplayNone, WhiteSpaceCollapse}},
		{source: `<table align=left></table>`, selector: `table`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<table align=center></table>`, selector: `table`, want: Whitespace{DisplayBlock, WhiteSpaceCollapse}},
		{source: `<div style="float:{{.F}}"></div>`, selector: `div`, want: Whitespace{DisplayUnknown, WhiteSpaceUnknown}},
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
			got, ok := ComputeWhitespace(trees, WhitespaceOptions{Context: context, Sheets: sheets, AuthorCSSKnown: test.known, MissingSheets: test.missing})[el]
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

// A <style> element whose CSS does not parse cleanly (a declaration list
// cut off after its colon, found by fuzzing) must not panic.
func TestComputeWhitespaceMalformedStyle(t *testing.T) {
	for _, source := range []string{
		`<stYle>{A:`,
		`<style>{A:`,
		`<style>p{A:`,
		`<style>p{display:`,
		`<style>{display:block`,
		`<style>p{a:b;c:</style><p>x</p>`,
	} {
		t.Run(source, func(t *testing.T) {
			trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", source)}
			got := ComputeWhitespace(trees, WhitespaceOptions{})
			if len(got) == 0 {
				t.Errorf("ComputeWhitespace(%q) classified no element", source)
			}
		})
	}
}

func TestWhiteSpaceMovedMatcher(t *testing.T) {
	const page = `<section class="post"><div id="moved"><pre></pre><b></b></div><span></span></section>`
	for _, tt := range []struct {
		sheet   string
		missing bool
		want    bool // MayMatch(the div with id moved)
	}{
		{sheet: ``, want: false},
		{sheet: `.k{white-space:pre}`, want: false},
		{sheet: `div{white-space:pre}`, want: false}, // no combinator: matched alike anywhere
		{sheet: `div:not(.x){white-space:pre}`, want: false},
		{sheet: `div:hover{white-space:pre}`, want: false},
		{sheet: `section > div{display:block}`, want: false},
		{sheet: `section div{color:red}`, want: false},
		{sheet: `.post code{white-space:pre}`, want: false},
		{sheet: `.post [data-x="a > b"]{white-space:pre}`, want: false},
		{sheet: `section > div{white-space:pre}`, want: true},
		{sheet: `.post pre{white-space:pre-line}`, want: true},
		{sheet: `div b{white-space-collapse:preserve}`, want: true},
		{sheet: `div + span{white-space:pre}`, want: true},
		{sheet: `div ~ code{white-space:pre}`, want: true}, // the moved div gets new later siblings
		{sheet: `b ~ code{white-space:pre}`, want: false},  // b keeps its siblings
		{sheet: `p ~ div{white-space:pre}`, want: true},
		{sheet: `.a, section div{white-space:pre}`, want: true},
		{sheet: `:is(.a, .b) div{white-space:pre}`, want: true},
		{sheet: `div:not(p div){white-space:pre}`, want: true},
		{sheet: `:where(.a > .b){white-space:pre}`, want: true},
		{sheet: `div:not(:is(p div)){white-space:pre}`, want: true},
		{sheet: `div:where(:not(p > div)){white-space:pre}`, want: true},
		{sheet: `span:is(:not(p + span)){white-space:pre}`, want: true},
		{sheet: `div:is(:not(.a)){white-space:pre}`, want: false},
		{sheet: `main:has(p){white-space:pre}`, want: true},
		{sheet: `li:nth-child(2){white-space:pre}`, want: true},
		{sheet: `p:first-child{white-space:pre}`, want: true},
		{sheet: `p:empty{white-space:pre}`, want: true},
		{sheet: `:root{white-space:pre}`, want: true},
		{sheet: `.post{& > div{all:unset}}`, want: true},
		{sheet: `.post{& code{all:unset}}`, want: false},
		{sheet: `@media print{section div{white-space:pre}}`, want: true},
		{sheet: ``, missing: true, want: true},
	} {
		tree := parseWhitespaceTree(t, "page", page)
		moved, _ := selectors.MustCompile(`#moved`).QuerySelector(tree.Root)
		if moved == nil {
			t.Fatal("no moved element")
		}
		sheets := []Stylesheet{{Filename: "site.css", Text: tt.sheet}}
		m := WhiteSpaceMovedMatcher(nil, WhitespaceOptions{Sheets: sheets, AuthorCSSKnown: true, MissingSheets: tt.missing})
		if got := m.MayMatch([]*template.HTMLElementNode{moved}); got != tt.want {
			t.Errorf("WhiteSpaceMovedMatcher(%q, missing %v).MayMatch(div#moved) = %v, want %v", tt.sheet, tt.missing, got, tt.want)
		}
	}
}

// TestComputeWhitespaceAncestry checks author selectors that depend on
// ancestors: an element of a define or block body has the ancestors of
// each call site (certain only when every site agrees, maybe with no
// site); an ancestor opened in a template branch, or whose end tag is in
// one, may or may not hold what follows, and so may the html and body
// elements a file without them is implied into.
func TestComputeWhitespaceAncestry(t *testing.T) {
	const bubble = `.bubble p{white-space:pre}`
	for _, test := range []struct {
		name    string
		source  string
		context string
		sheet   string
		want    WhiteSpace // of the first p
	}{
		{name: "call site matches", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}<div class="bubble">{{template "m"}}</div>`, want: WhiteSpacePreserve},
		{name: "call site does not match", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}<div class="x">{{template "m"}}</div>`, want: WhiteSpaceCollapse},
		{name: "call sites disagree", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}<div class="bubble">{{template "m"}}</div><div>{{template "m"}}</div>`, want: WhiteSpaceUnknown},
		{name: "no call site", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}<div></div>`, want: WhiteSpaceUnknown},
		{name: "call site in a context template", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}`, context: `<div class="bubble">{{template "m"}}</div>`, want: WhiteSpacePreserve},
		{name: "through another define", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}{{define "n"}}<b>{{template "m"}}</b>{{end}}<div class="bubble">{{template "n"}}</div>`, want: WhiteSpacePreserve},
		{name: "child through a call site", sheet: `.bubble>p{white-space:pre}`, source: `{{define "m"}}<p></p>{{end}}<div class="bubble">{{template "m"}}</div>`, want: WhiteSpacePreserve},
		{name: "child not through a wrapper", sheet: `.bubble>p{white-space:pre}`, source: `{{define "m"}}<p></p>{{end}}<div class="bubble"><b>{{template "m"}}</b></div>`, want: WhiteSpaceCollapse},
		{name: "recursive call", sheet: bubble, source: `{{define "m"}}<p></p>{{template "m"}}{{end}}<div class="bubble">{{template "m"}}</div>`, want: WhiteSpaceUnknown},
		{name: "block in place", sheet: bubble, source: `<div class="bubble">{{block "b" .}}<p></p>{{end}}</div>`, want: WhiteSpacePreserve},
		{name: "ancestor opened in a branch", sheet: bubble, source: `{{if .}}<div class="bubble">{{else}}<div>{{end}}<p></p></div>`, want: WhiteSpaceUnknown},
		{name: "ancestor opened in a branch before a call", sheet: bubble, source: `{{define "m"}}<p></p>{{end}}{{if .}}<div class="bubble">{{else}}<div>{{end}}{{template "m"}}</div>`, want: WhiteSpaceUnknown},
		{name: "parent opened in a branch", sheet: `.bubble>p{white-space:pre}`, source: `{{if .}}<div class="bubble">{{end}}<p></p>{{if .}}</div>{{end}}`, want: WhiteSpaceUnknown},
		{name: "ancestors opened in a branch match nothing", sheet: bubble, source: `{{if .}}<div class="x">{{else}}<div class="y">{{end}}<p></p></div>`, want: WhiteSpaceCollapse},
		{name: "ancestor closed in a branch", sheet: bubble, source: `<div class="bubble">{{if .}}</div>{{end}}<p></p>`, want: WhiteSpaceUnknown},
		{name: "a closed ancestor before a branch", sheet: bubble, source: `<div class="bubble">{{if .}}<b>x</b>{{end}}<p></p></div>`, want: WhiteSpacePreserve},
		{name: "implied body", sheet: `body p{white-space:pre}`, source: `<div><p></p></div>`, want: WhiteSpaceUnknown},
		{name: "implied body in a define", sheet: `body p{white-space:pre}`, source: `{{define "m"}}<p></p>{{end}}<div>{{template "m"}}</div>`, want: WhiteSpaceUnknown},
		{name: "body", sheet: `body p{white-space:pre}`, source: `<html><body><div><p></p></div></body></html>`, want: WhiteSpacePreserve},
		{name: "a class on the implied body", sheet: `.dark p{white-space:pre}`, source: `<div><p></p></div>`, want: WhiteSpaceCollapse},
		{name: "child of the implied body", sheet: `div>p{white-space:pre}`, source: `<p></p>`, want: WhiteSpaceCollapse},
	} {
		t.Run(test.name, func(t *testing.T) {
			trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", test.source)}
			options := WhitespaceOptions{AuthorCSSKnown: true, Sheets: []Stylesheet{{Filename: "site.css", Text: test.sheet}}}
			if test.context != "" {
				options.Context = []*template.Tree{parseWhitespaceTree(t, "c.gohtml", test.context)}
			}
			got := ComputeWhitespace(trees, options)[queryElement(t, trees, "p")]
			if got.WhiteSpace != test.want {
				t.Errorf("ComputeWhitespace(%q)[p].WhiteSpace = %v, want %v", test.source, got.WhiteSpace, test.want)
			}
		})
	}
}

// BenchmarkComputeWhitespaceCallSites classifies a file of 50 defines,
// each of 20 elements nested four deep and called from the 5 defines
// after it (so the ways up from an element multiply), under 60
// descendant white-space rules.
func BenchmarkComputeWhitespaceCallSites(b *testing.B) {
	trees, options := callSitesBenchmarkInput(b)
	b.ResetTimer()
	for range b.N {
		ComputeWhitespace(trees, options)
	}
}

func callSitesBenchmarkInput(tb testing.TB) ([]*template.Tree, WhitespaceOptions) {
	tb.Helper()
	const defines, sites = 50, 5
	var src, css strings.Builder
	for i := range defines {
		fmt.Fprintf(&src, `{{define "d%d"}}`, i)
		for k := range 5 {
			fmt.Fprintf(&src, `<div class="c%d"><section><article><p>x</p></article></section></div>`, (i+k)%60)
		}
		for k := 1; k <= sites && i+k < defines; k++ {
			if k == 1 {
				fmt.Fprintf(&src, `<div class="c%d">{{template "d%d"}}</div>`, i%60, i+k)
			} else {
				fmt.Fprintf(&src, `{{template "d%d"}}`, i+k)
			}
		}
		src.WriteString(`{{end}}`)
	}
	src.WriteString(`<main>{{template "d0"}}</main>`)
	for r := range 60 {
		fmt.Fprintf(&css, ".c%d p{white-space:pre}\n", r)
	}
	tree, err := template.ParseTree("bench.gohtml", "bench.gohtml", src.String(), nil, lex.NewOptions())
	if err != nil {
		tb.Fatal(err)
	}
	return []*template.Tree{tree}, WhitespaceOptions{AuthorCSSKnown: true, Sheets: []Stylesheet{{Filename: "site.css", Text: css.String()}}}
}
