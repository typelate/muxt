package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// autoClosedBy maps an open element to the start tags that make
// browsers close it first (WHATWG §13.2.6.4.7, .13, .14, .15).
var autoClosedBy = map[string]map[string]bool{
	"p": setOf("address", "article", "aside", "blockquote", "center",
		"details", "dialog", "dir", "div", "dl", "fieldset", "figcaption",
		"figure", "footer", "header", "hgroup", "main", "menu", "nav",
		"ol", "p", "search", "section", "summary", "ul",
		"h1", "h2", "h3", "h4", "h5", "h6", "pre", "listing", "form",
		"li", "dd", "dt", "plaintext", "table", "hr", "xmp"),
	"li":       setOf("li"),
	"dd":       setOf("dd", "dt"),
	"dt":       setOf("dd", "dt"),
	"option":   setOf("option", "optgroup"),
	"optgroup": setOf("optgroup"),
	"td":       setOf("caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr"),
	"th":       setOf("caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr"),
	"tr":       setOf("tr", "caption", "col", "colgroup", "tbody", "tfoot", "thead"),
	"tbody":    setOf("caption", "col", "colgroup", "tbody", "tfoot", "thead"),
	"tfoot":    setOf("caption", "col", "colgroup", "tbody", "tfoot", "thead"),
	"thead":    setOf("caption", "col", "colgroup", "tbody", "tfoot", "thead"),
}

// h1..h6 all share the same closer set (any other heading level);
// generated here rather than repeated six times in the literal above.
func init() {
	headers := setOf("h1", "h2", "h3", "h4", "h5", "h6")
	for h := range headers {
		autoClosedBy[h] = headers
	}
}

// buttonScopeBoundaries approximates the HTML "button scope" used by
// the <p> auto-close rule (§13.2.6.4.7): an ancestor scan for an open
// <p> stops here rather than crossing into an unrelated subtree.
var buttonScopeBoundaries = map[string]bool{
	"html": true, "table": true, "td": true, "th": true, "caption": true,
	"applet": true, "object": true, "marquee": true, "template": true, "button": true,
}

// setOf builds a lookup set from tags.
func setOf(tags ...string) map[string]bool {
	s := make(map[string]bool, len(tags))
	for _, tag := range tags {
		s[tag] = true
	}
	return s
}

// checkAutoClose reports elements whose start tag makes the browser
// close an open ancestor before this element is parsed (WHATWG's
// implied end tags, §13.2.6.4.7, .13, .14, .15). htt's AST nests the
// two elements literally as written; the browser DOM does not — which
// matters wherever htt reads element nesting (selectors, coverage).
//
// Every row except <p> checks only the direct enclosing element: the
// parser nests these literally (e.g. a second <li> lands INSIDE the
// first), so a direct check alone finds the auto-close without the
// false positive of `<li><ul><li>` (the inner <li>'s direct parent is
// <ul>, which is legal). The <p> row instead scans ancestors up to a
// button-scope boundary, because block content can auto-close a <p>
// through intervening inline elements such as <span>.
func checkAutoClose(t *template.Tree, diags *Diagnostics) {
	template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		tag := strings.ToLower(el.TagName)

		if enc, mod := enclosingElement(path); enc != nil && autoClosedBy[strings.ToLower(enc.TagName)][tag] {
			appendDiag(diags, el.Start(), el.End(), template.DiagAutoClosed, mod,
				fmt.Sprintf(msgAutoClosed, enc.TagName, el.TagName))
			return
		}

		if !autoClosedBy["p"][tag] {
			return
		}
		for anc, mod := range elementAncestors(path) {
			ancTag := strings.ToLower(anc.TagName)
			if ancTag == "p" {
				appendDiag(diags, el.Start(), el.End(), template.DiagAutoClosed, mod,
					fmt.Sprintf(msgAutoClosed, anc.TagName, el.TagName))
				return
			}
			if buttonScopeBoundaries[ancTag] {
				return
			}
		}
	})
}
