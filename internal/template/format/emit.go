package format

import (
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// emitter reindents a template (v2 spec §3 items 1–4, 8, 9). It writes
// the source as it is except for the whitespace after the last line
// break of a run it may reindent, which becomes one tab per depth
// level. It never joins or splits a line and never edits a token: every
// byte it writes that is not such a tab is copied from the source, so
// tokens (actions, trim markers, tag names, void slashes, attribute
// values) and runs the browser renders are as written by construction.
//
// Which runs it may reindent comes from the classifier (classify): a run
// the browser collapses (runReflowable, runCollapseOnly), a pre-line run
// (runPreserveBreaks: its line breaks are kept, only the spaces after
// the last one change), and a run a trim marker removes (runTrimmed: only
// its line breaks are kept). A run without a line break is never
// touched, so a run's kind (empty, spaces, line breaks) never changes.
// Whitespace between attributes inside a start tag is reindented too.
//
// Depth follows the source's tag structure (§3.3): define bodies at 0;
// branch bodies one deeper than their keywords; element children one
// deeper than the element, except that html adds no depth; optional end
// tags are implied (impliedEnd); an unclosed element closes where its
// parent does; a branch body's depth change ends at its {{end}}.
// Actions, template calls and blocks are transparent.
type emitter struct {
	src       string
	base      lex.Pos // the position of src[0]
	cls       classes
	opaque    map[template.Node]bool
	asWritten map[*template.DefineNode]bool // define bodies left as written
	verbatim  [][2]int                      // source spans never edited: muxt:fmt regions
	edits     []edit
}

// edit replaces src[start:end], a whitespace run, with text.
type edit struct {
	start, end int
	text       string
}

// openOptional is an element whose end tag the source leaves out and
// the HTML parser may imply (impliedEnd), with the depth it sits at.
type openOptional struct {
	tag   string
	depth int
}

func newEmitter(src string, base lex.Pos, cls classes, asWritten map[*template.DefineNode]bool, dir *directives) *emitter {
	e := &emitter{
		src:       src,
		base:      base,
		cls:       cls,
		opaque:    make(map[template.Node]bool, len(cls.opaque)),
		asWritten: asWritten,
	}
	for _, n := range cls.opaque {
		e.opaque[n] = true
	}
	if dir != nil {
		for c, r := range dir.regions {
			e.verbatim = append(e.verbatim, [2]int{dir.commentEnd(c) - int(base), r.end - int(base)})
		}
	}
	return e
}

// emit returns the source of root with its edits applied.
func (e *emitter) emit(root *template.ListNode) string {
	// Nothing closes the root: its last run is the end of the file,
	// which is kept as written (closeDepth -1).
	e.list(root, root.Nodes, 0, -1, nil)
	slices.SortFunc(e.edits, func(a, b edit) int { return a.start - b.start })
	var b strings.Builder
	b.Grow(len(e.src))
	at := 0
	for _, ed := range e.edits {
		if ed.start < at || e.inVerbatim(ed.start) {
			continue
		}
		b.WriteString(e.src[at:ed.start])
		b.WriteString(ed.text)
		at = ed.end
	}
	b.WriteString(e.src[at:])
	return b.String()
}

func (e *emitter) inVerbatim(offset int) bool {
	for _, span := range e.verbatim {
		if span[0] <= offset && offset < span[1] {
			return true
		}
	}
	return false
}

// list walks the child list key, whose items sit at depth and whose
// last run is followed by what closes the list at closeDepth (an end
// tag, {{else}}, {{end}}); closeDepth is -1 when nothing does. open are
// the unclosed elements with optional end tags whose content continues
// in this list, innermost last.
func (e *emitter) list(key template.Node, children []template.Node, depth, closeDepth int, open []openOptional) {
	items, _ := splitChildren(children)
	for i, item := range items {
		if el, ok := item.(*template.HTMLElementNode); ok {
			depth, open = impliedEnds(strings.ToLower(el.TagName), depth, open)
		}
		if !e.leftWhole(item) {
			e.gap(key, i, depth)
		}
		e.item(item, depth, closeDepth, open)
	}
	e.gap(key, len(items), closeDepth)
}

func (e *emitter) gap(key template.Node, i, depth int) {
	if g, ok := e.cls.gaps[gapKey{key, i}]; ok {
		e.reindent(g, depth)
	}
}

// item walks n at depth. after is the depth of what follows the list n
// is in, where an unclosed element's content ends.
func (e *emitter) item(n template.Node, depth, after int, open []openOptional) {
	switch n := n.(type) {
	case *template.TextNode:
		for _, r := range e.cls.runs[n] {
			e.reindent(r, depth)
		}
	case *template.HTMLElementNode:
		e.element(n, depth, after, open)
	case *template.IfNode:
		e.branch(&n.BranchNode, depth)
	case *template.WithNode:
		e.branch(&n.BranchNode, depth)
	case *template.RangeNode:
		e.branch(&n.BranchNode, depth)
	case *template.DefineNode:
		if n.Body != nil && !e.asWritten[n] {
			e.list(n.Body, n.Body.Nodes, 0, 0, nil)
		}
	case *template.BlockNode:
		if n.Body != nil {
			e.list(n.Body, n.Body.Nodes, depth, depth, nil)
		}
	}
}

// branch walks b's body and else list one level deeper than its
// keywords. A chained {{else if}} is a keyword of b, so the branch it
// opens sits at b's depth.
func (e *emitter) branch(b *template.BranchNode, depth int) {
	if b.List != nil {
		e.list(b.List, b.List.Nodes, depth+1, depth, nil)
	}
	if b.ElseList != nil {
		inner := depth + 1
		if b.ElseChain() != nil {
			inner = depth
		}
		e.list(b.ElseList, b.ElseList.Nodes, inner, depth, nil)
	}
}

func (e *emitter) element(el *template.HTMLElementNode, depth, after int, open []openOptional) {
	if e.opaque[el] {
		return // as written, start tag included (spec §3 item 4)
	}
	e.startTag(el, depth)
	if el.Void {
		return
	}
	tag := strings.ToLower(el.TagName)
	inner := depth + 1
	if tag == "html" {
		inner = depth
	}
	if el.Closed {
		e.list(el, el.Children, inner, depth, nil)
		return
	}
	// The content of an unclosed element runs to its parent's end.
	var childOpen []openOptional
	if _, ok := optionalEndClosers[tag]; ok {
		childOpen = append(slices.Clone(open), openOptional{tag: tag, depth: depth})
	}
	e.list(el, el.Children, inner, after, childOpen)
}

// leftWhole reports whether n is left as written over more than one
// line: an element the formatter does not reindent inside (opaque) or
// a define body left as written. The run before it, which starts its
// first line, is left as written too, so the element keeps its own
// columns (spec §3 item 4). One that fits on a line has its line
// reindented.
func (e *emitter) leftWhole(n template.Node) bool {
	switch n := n.(type) {
	case *template.HTMLElementNode:
		return e.opaque[n] && e.spansLines(n.Start(), elementEnd(n))
	case *template.DefineNode:
		return e.asWritten[n] && e.spansLines(n.Start(), n.End())
	}
	return false
}

func (e *emitter) spansLines(start, end lex.Pos) bool {
	from, to := int(start-e.base), int(end-e.base)
	if from < 0 || to > len(e.src) || from > to {
		return true
	}
	return strings.ContainsAny(e.src[from:to], "\n\r")
}

// elementEnd is the end of el's source: its end tag, or for an element
// the source leaves unclosed, its last descendant.
func elementEnd(el *template.HTMLElementNode) lex.Pos {
	end := el.End()
	if el.Closed || el.Void {
		return end
	}
	if flat := flatten(el.Children); len(flat) > 0 {
		last := flat[len(flat)-1]
		if child, ok := last.(*template.HTMLElementNode); ok {
			return max(end, elementEnd(child))
		}
		return max(end, last.End())
	}
	return end
}

// reindent records the edit for run g at depth: when g holds a line
// break and its class lets the formatter change it, the whitespace
// after its last line break becomes depth tabs. A trimmed run, which
// never renders, keeps only its line breaks. Every other run, and
// every run at depth -1, is left as written.
func (e *emitter) reindent(g gap, depth int) {
	if g.text == nil || depth < 0 {
		return
	}
	run := g.text.Text[g.start:g.end]
	last := afterLastBreak(run)
	if last < 0 {
		return
	}
	var kept []byte
	switch g.class {
	case runTrimmed:
		kept = lineBreakBytes(run)
	case runReflowable, runCollapseOnly, runPreserveBreaks:
		kept = run[:last]
	default:
		return
	}
	start := int(g.text.Start()-e.base) + g.start
	end := start + len(run)
	if start < 0 || end > len(e.src) || e.src[start:end] != string(run) {
		return // the text is not the source's: leave it
	}
	e.edits = append(e.edits, edit{start: start, end: end, text: string(kept) + strings.Repeat("\t", depth)})
}

// startTag reindents the continuation lines of el's start tag (spec §3
// item 8): the whitespace after the last line break of a run between
// attributes becomes depth+1 tabs, followed by the spaces the source has
// after its last tab there (alignment). A > or /> on a line of its own
// goes at the tag's depth, without alignment. Runs without a line break, attribute values
// and the slash are as written. A start tag that is not a list of
// attributes and actions separated by whitespace is as written.
func (e *emitter) startTag(el *template.HTMLElementNode, depth int) {
	from, to := int(el.AttrStart-e.base), int(el.AttrEnd-e.base)
	if from < 0 || to > len(e.src) || to <= from || e.src[to-1] != '>' {
		return
	}
	type tagRun struct {
		start, end, depth int
		close             bool // the run before > or />
	}
	var runs []tagRun
	prev := from
	for _, attr := range el.Attr {
		switch attr.(type) {
		case *template.HTMLAttributeNode, *template.ActionNode, *template.TemplateNode:
		default:
			return
		}
		at, end := int(attr.Start()-e.base), int(attr.End()-e.base)
		if at < prev || end < at || end > to-1 {
			return
		}
		before := e.src[prev:at]
		if strings.Trim(before, tagWhitespace) != "" || before == "" && prev == from {
			return
		}
		runs = append(runs, tagRun{start: prev, end: at, depth: depth + 1})
		prev = end
	}
	tail := e.src[prev : to-1]
	if strings.Count(tail, "/") > 1 || strings.Trim(tail, tagWhitespace+"/") != "" {
		return
	}
	if slash := strings.IndexByte(tail, '/'); slash >= 0 {
		runs = append(runs, tagRun{start: prev, end: prev + slash, depth: depth, close: true})
	} else {
		runs = append(runs, tagRun{start: prev, end: to - 1, depth: depth, close: true})
	}
	for _, r := range runs {
		run := e.src[r.start:r.end]
		last := afterLastBreak([]byte(run))
		if last < 0 {
			continue
		}
		var align string
		if !r.close {
			indent := run[last:]
			align = indent[strings.LastIndexByte(indent, '\t')+1:]
		}
		e.edits = append(e.edits, edit{start: r.start, end: r.end, text: run[:last] + strings.Repeat("\t", r.depth) + align})
	}
}

// tagWhitespace is the whitespace the HTML tokenizer skips between
// attributes.
const tagWhitespace = " \t\n\r\f"

// afterLastBreak returns the index after the last line break (LF or CR)
// in run, or -1 when it has none.
func afterLastBreak(run []byte) int {
	for i := len(run) - 1; i >= 0; i-- {
		if run[i] == '\n' || run[i] == '\r' {
			return i + 1
		}
	}
	return -1
}

// lineBreakBytes returns the LF and CR bytes of run, in order.
func lineBreakBytes(run []byte) []byte {
	var kept []byte
	for _, b := range run {
		if b == '\n' || b == '\r' {
			kept = append(kept, b)
		}
	}
	return kept
}

// impliedEnds closes the open elements a start tag named tag implies the
// end of (impliedEnd), innermost first, and returns the depth the tag
// sits at and the elements still open.
func impliedEnds(tag string, depth int, open []openOptional) (int, []openOptional) {
	for len(open) > 0 && impliedEnd(open[len(open)-1].tag, tag) {
		depth = open[len(open)-1].depth
		open = open[:len(open)-1]
	}
	return depth, open
}

// impliedEnd reports whether a start tag named start ends an open
// element named open whose end tag the source leaves out (HTML §13.1.2.4
// optional end tags; §13.2.6.4.7 "generate implied end tags" and "close
// a p element"). It is used for depth only, so it errs on simple.
func impliedEnd(open, start string) bool {
	closers, ok := optionalEndClosers[open]
	if !ok {
		return false
	}
	if open == "colgroup" {
		return start != "col" && start != "template"
	}
	return closers[start]
}

func tagSet(tags ...string) map[string]bool {
	set := make(map[string]bool, len(tags))
	for _, t := range tags {
		set[t] = true
	}
	return set
}

var (
	cellClosers    = tagSet("td", "th", "tr", "tbody", "thead", "tfoot")
	sectionClosers = tagSet("tbody", "thead", "tfoot")
)

// optionalEndClosers are, for each element whose end tag may be left
// out, the start tags that end it. colgroup is ended by any start tag
// but col (impliedEnd).
var optionalEndClosers = map[string]map[string]bool{
	"li": tagSet("li"),
	"dt": tagSet("dt", "dd"),
	"dd": tagSet("dt", "dd"),
	"p": tagSet("address", "article", "aside", "blockquote", "center", "details", "dialog",
		"dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "form",
		"h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr", "listing",
		"main", "menu", "nav", "ol", "p", "plaintext", "pre", "search", "section",
		"summary", "table", "ul", "xmp", "li", "dd", "dt"),
	"option":   tagSet("option", "optgroup", "hr"),
	"tr":       tagSet("tr", "tbody", "thead", "tfoot"),
	"td":       cellClosers,
	"th":       cellClosers,
	"thead":    sectionClosers,
	"tbody":    sectionClosers,
	"tfoot":    sectionClosers,
	"colgroup": nil,
	"caption":  tagSet("caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr"),
}
