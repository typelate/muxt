package format

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// emitter reindents a template (v2 spec §3 items 1–6, 8, 9). It writes
// the source as it is except for the whitespace of a run it may edit
// that holds a line break: the run keeps its line breaks, drops the
// spaces and tabs before them, and ends in one tab per depth level. It
// never joins or splits a line and never edits a token: every byte it
// writes that is not such a tab is copied from the source, so tokens
// (actions, trim markers, tag names, void slashes, attribute values) and
// runs the browser renders exactly are as written by construction.
//
// Which runs it may edit comes from the classifier (classify): a run
// the browser collapses (runCollapse), where two or
// more blank lines also become one; a pre-line run (runPreserveBreaks),
// whose line breaks are all kept; and a run a trim marker removes
// (runTrimmed), treated as a collapsed one. A run without a line break
// is never touched, so a run's kind (empty, spaces, line breaks) never
// changes. Whitespace between attributes inside a start tag is
// reindented too.
//
// Depth follows the source's tag structure (§3.3): define bodies at 0;
// branch bodies one deeper than their keywords; element children one
// deeper than the element, except that html adds no depth; optional end
// tags are implied (impliedEnd); an unclosed element closes where its
// parent does; a branch body's depth change ends at its {{end}}.
// Actions, template calls and blocks are transparent; a define is at
// column 0 wherever it sits. A child list with a line left as written
// for want of its white-space is as written whole, subtree included
// (list), and a multi-line comment's continuation lines move with its
// first line (shiftComment).
type emitter struct {
	src             string
	base            lex.Pos // the position of src[0]
	cls             classes
	opaque          map[template.Node]bool
	asWrittenDefine map[*template.DefineNode]bool // define bodies left as written
	verbatim        [][2]int                      // source spans never edited: muxt:fmt regions
	frozenBranches  map[*template.BranchNode]bool // memo of frozenBranch
	edits           []edit
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
		src:             src,
		base:            base,
		cls:             cls,
		opaque:          make(map[template.Node]bool, len(cls.opaque)),
		asWrittenDefine: asWritten,
		frozenBranches:  make(map[*template.BranchNode]bool),
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

// emit returns the source of root with its edits applied. The edits
// cover disjoint runs by construction; an overlap is a formatter bug,
// reported as an error rather than written.
func (e *emitter) emit(root *template.ListNode) (string, error) {
	// Nothing closes the root: its last run is the end of the file,
	// whose last line is kept as written (closeDepth -1).
	e.list(root, root.Nodes, 0, -1, nil, false, false)
	slices.SortStableFunc(e.edits, func(a, b edit) int { return a.start - b.start })
	var b strings.Builder
	b.Grow(len(e.src))
	at := 0
	for _, ed := range e.edits {
		if ed.start < at {
			return "", fmt.Errorf("%w: offset %d", errOverlappingEdits, int(e.base)+ed.start)
		}
		if e.inVerbatim(ed.start) {
			continue
		}
		b.WriteString(e.src[at:ed.start])
		b.WriteString(ed.text)
		at = ed.end
	}
	b.WriteString(e.src[at:])
	return b.String(), nil
}

// errOverlappingEdits is the error emit returns when two of its edits
// overlap.
var errOverlappingEdits = errors.New("format: overlapping whitespace edits")

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
// in this list, innermost last. frozen reports that the list is inside
// a subtree left as written, where nothing is edited; closeAsWritten
// that the run before what closes it is as written (a multi-line
// {{else}} or {{end}}).
//
// A list whose white-space the formatter cannot tell is all or nothing
// (frozenList): when one of its lines must stay as written for that
// reason, every run that starts a line of it does, trimmed and
// collapsing ones included, and so does the whole subtree of each of
// its items. What closes the list still lands at its depth. A branch
// whose body is so frozen is as written whole, keyword lines included
// (frozenBranch); its siblings reindent. An item left whole by nature
// (leftWhole: pre, script, a custom element, a multi-line action or
// opener) is an island: its first line is as written and its siblings
// reindent. A {{define}} is a top-level declaration wherever it sits:
// its opener, body and {{end}} are at column 0 whatever list it is in.
func (e *emitter) list(key template.Node, children []template.Node, depth, closeDepth int, open []openOptional, frozen, closeAsWritten bool) {
	items, _ := splitChildren(children)
	own := !frozen && e.frozenList(key, items)
	for i, item := range items {
		if el, ok := item.(*template.HTMLElementNode); ok {
			depth, open = impliedEnds(strings.ToLower(el.TagName), depth, open)
		}
		d := depth
		_, define := item.(*template.DefineNode)
		if define {
			d = 0
		}
		sub := frozen || own
		island := e.leftWhole(key, i, item, d)
		if b := branchOf(item); b != nil && !sub && e.frozenBranch(b) {
			island, sub = true, true
		}
		if !island && (define || !frozen && !own) {
			if g, ok := e.cls.gaps[gapKey{key, i}]; ok {
				e.reindent(g, d)
				if c, ok := item.(*template.CommentNode); ok {
					e.shiftComment(c, g, d)
				}
			}
		}
		if _, text := item.(*template.TextNode); text && sub {
			continue
		}
		e.item(item, d, closeDepth, open, sub && !define, frozen || closeAsWritten)
	}
	if !frozen && !closeAsWritten {
		e.gap(key, len(items), closeDepth)
	}
}

// frozenList reports whether the list key has a line that must stay as
// written because the formatter cannot tell its white-space (cls.unsure):
// a preserved run starting a line of a list whose white-space is
// unknown, or a multi-line unsure element wherever it starts. Defines
// do not count: they are not part of the list's layout.
func (e *emitter) frozenList(key template.Node, items []template.Node) bool {
	unsure := e.cls.unsure[key]
	for _, item := range items {
		if el, ok := item.(*template.HTMLElementNode); ok && e.cls.unsure[el] && e.spansLines(el.Start(), elementEnd(el)) {
			return true
		}
	}
	for i := range len(items) + 1 {
		if g, ok := e.cls.gaps[gapKey{key, i}]; ok && unsure && g.class == runPreserved && startsLine(g) {
			return true
		}
	}
	if !unsure {
		return false
	}
	for _, item := range items {
		if text, ok := item.(*template.TextNode); ok {
			for _, r := range e.cls.runs[text] {
				if r.class == runPreserved && startsLine(r) {
					return true
				}
			}
		}
	}
	return false
}

// frozenBranch reports whether one of b's bodies, through a chained
// {{else if}}, is frozen (frozenBody). A frozen branch freezes the
// branch bodies around it, up to the nearest element, define or block
// body, where it is an island.
func (e *emitter) frozenBranch(b *template.BranchNode) bool {
	if frozen, ok := e.frozenBranches[b]; ok {
		return frozen
	}
	frozen := e.frozenBody(b.List)
	if !frozen && b.ElseList != nil {
		if chain := b.ElseChain(); chain != nil {
			frozen = e.frozenBranch(chain)
		} else {
			frozen = e.frozenBody(b.ElseList)
		}
	}
	e.frozenBranches[b] = frozen
	return frozen
}

// frozenBody reports whether body, a branch's list, is frozen: a frozen
// list (frozenList) or one holding a frozen branch.
func (e *emitter) frozenBody(body *template.ListNode) bool {
	if body == nil {
		return false
	}
	items := listItems(body)
	if e.frozenList(body, items) {
		return true
	}
	for _, item := range items {
		if b := branchOf(item); b != nil && e.frozenBranch(b) {
			return true
		}
	}
	return false
}

// startsLine reports whether run g holds a line break, so that what
// follows it starts a line.
func startsLine(g gap) bool {
	return g.text != nil && afterLastBreak(g.text.Text[g.start:g.end]) >= 0
}

func (e *emitter) gap(key template.Node, i, depth int) {
	if g, ok := e.cls.gaps[gapKey{key, i}]; ok {
		e.reindent(g, depth)
	}
}

// item walks n at depth. after is the depth of what follows the list n
// is in, where an unclosed element's content ends. frozen reports that
// n's subtree is left as written; afterAsWritten that the run before
// what follows the list n is in is as written, which an unclosed
// element's content shares.
func (e *emitter) item(n template.Node, depth, after int, open []openOptional, frozen, afterAsWritten bool) {
	switch n := n.(type) {
	case *template.TextNode:
		for _, r := range e.cls.runs[n] {
			e.reindent(r, depth)
		}
	case *template.HTMLElementNode:
		e.element(n, depth, after, open, frozen, afterAsWritten)
	case *template.IfNode:
		e.branch(&n.BranchNode, depth, frozen)
	case *template.WithNode:
		e.branch(&n.BranchNode, depth, frozen)
	case *template.RangeNode:
		e.branch(&n.BranchNode, depth, frozen)
	case *template.DefineNode:
		if n.Body != nil && !e.asWrittenDefine[n] {
			e.list(n.Body, n.Body.Nodes, 0, 0, nil, false, false)
		}
	case *template.BlockNode:
		if n.Body != nil {
			e.list(n.Body, n.Body.Nodes, depth, depth, nil, frozen, false)
		}
	}
}

// branch walks b's body and else list one level deeper than its
// keywords. A chained {{else if}} is a keyword of b, so the branch it
// opens sits at b's depth. frozen reports that b is left as written. An
// {{else}} or {{end}} spanning lines keeps the run before it as
// written, like a multi-line action (leftWhole).
func (e *emitter) branch(b *template.BranchNode, depth int, frozen bool) {
	if b.List == nil {
		return
	}
	chain := b.ElseChain()
	last := b.List
	if b.ElseList != nil {
		last = b.ElseList
	}
	endSpans := chain == nil && e.spansLines(last.End(), b.End())
	if b.ElseList == nil {
		e.list(b.List, b.List.Nodes, depth+1, depth, nil, frozen, endSpans)
		return
	}
	elseEnd := b.ElseList.Start()
	if chain != nil && chain.List != nil {
		elseEnd = chain.List.Start()
	}
	e.list(b.List, b.List.Nodes, depth+1, depth, nil, frozen, e.spansLines(b.List.End(), elseEnd))
	inner := depth + 1
	if chain != nil {
		inner = depth
	}
	e.list(b.ElseList, b.ElseList.Nodes, inner, depth, nil, frozen, endSpans)
}

func (e *emitter) element(el *template.HTMLElementNode, depth, after int, open []openOptional, frozen, afterAsWritten bool) {
	if e.opaque[el] {
		return // as written, start tag included (spec §3 item 4)
	}
	if !frozen {
		e.startTag(el, depth)
	}
	if el.Void {
		return
	}
	tag := strings.ToLower(el.TagName)
	inner := depth + 1
	if tag == "html" {
		inner = depth
	}
	if el.Closed {
		e.list(el, el.Children, inner, depth, nil, frozen, false)
		return
	}
	// The content of an unclosed element runs to its parent's end. The
	// optional-end elements open around it stay open inside it, as far
	// as the start tags that end them reach past it (reachesPast).
	childOpen := slices.DeleteFunc(slices.Clone(open), func(o openOptional) bool {
		return !reachesPast(o.tag, tag)
	})
	if _, ok := optionalEndClosers[tag]; ok {
		childOpen = append(childOpen, openOptional{tag: tag, depth: depth})
	}
	e.list(el, el.Children, inner, after, childOpen, frozen, afterAsWritten)
}

// leftWhole reports whether n, item i of the list key at depth, is left
// as written over more than one line: an element the formatter does not
// reindent inside (opaque); a define body left as written; an action,
// template call or branch, define or block opener spanning lines, whose
// string literals may hold line breaks; or a {{/* */}} comment whose
// continuation lines cannot shift by its first line's change
// (shiftComment). The run before it, which starts its first line, is
// left as written too, so it keeps its own columns (spec §3 item 4). An
// element or define that fits on a line has its line reindented.
func (e *emitter) leftWhole(key template.Node, i int, n template.Node, depth int) bool {
	switch n := n.(type) {
	case *template.HTMLElementNode:
		return e.opaque[n] && e.spansLines(n.Start(), elementEnd(n))
	case *template.DefineNode:
		return e.asWrittenDefine[n] && e.spansLines(n.Start(), n.End()) ||
			n.Body != nil && e.spansLines(n.Start(), n.Body.Start())
	case *template.BlockNode:
		return n.Body != nil && e.spansLines(n.Start(), n.Body.Start())
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		b := branchOf(n)
		return b.List != nil && e.spansLines(b.Start(), b.List.Start())
	case *template.ActionNode, *template.TemplateNode, *template.BreakNode, *template.ContinueNode:
		return e.spansLines(n.Start(), n.End())
	case *template.CommentNode:
		g, ok := e.cls.gaps[gapKey{key, i}]
		if !ok {
			return false
		}
		_, ok = e.commentShifts(n, g, depth)
		return !ok
	}
	return false
}

// shiftComment moves the continuation lines of comment c, whose first
// line run g starts, by the change reindent makes to that line at depth
// (spec §3 item 3): the old indentation at the start of each line that is
// not blank becomes the new one. The comment is never rendered; only the
// leading whitespace of its continuation lines changes, so hanging
// alignment inside it survives.
func (e *emitter) shiftComment(c *template.CommentNode, g gap, depth int) {
	shifts, _ := e.commentShifts(c, g, depth)
	e.edits = append(e.edits, shifts...)
}

// commentShifts returns the edits shiftComment makes, and false when a
// continuation line cannot move by exactly the change: it does not start
// with the old indentation and the new one does not extend it.
func (e *emitter) commentShifts(c *template.CommentNode, g gap, depth int) ([]edit, bool) {
	from, to := int(c.Start()-e.base), int(c.End()-e.base)
	if from < 0 || to > len(e.src) || from > to {
		return nil, true
	}
	old, indent, moved := e.indents(g, depth)
	if !moved || old == indent || !strings.ContainsAny(e.src[from:to], "\n\r") {
		return nil, true
	}
	var shifts []edit
	for at := from; at < to; at++ {
		if b := e.src[at]; b != '\n' && (b != '\r' || at+1 < to && e.src[at+1] == '\n') {
			continue
		}
		line := at + 1
		rest := e.src[line:to]
		if strings.Trim(lineOf(rest), " \t") == "" {
			continue // a blank line: nothing to align
		}
		switch {
		case strings.HasPrefix(rest, old):
			shifts = append(shifts, edit{start: line, end: line + len(old), text: indent})
		case strings.HasPrefix(indent, old):
			shifts = append(shifts, edit{start: line, end: line, text: indent[len(old):]})
		default:
			return nil, false
		}
	}
	return shifts, true
}

// lineOf returns s up to its first line break.
func lineOf(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		return s[:i]
	}
	return s
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

// reindent records the edit for run g at depth (spec §3 items 2, 5, 6)
// when g holds a line break and its class lets the formatter change it.
// The run keeps its line breaks and loses the spaces and tabs before
// them (trailing whitespace and blank lines' indentation); under
// collapse, and in a trimmed run, which never renders, at most two line
// breaks are kept (one blank line). The whitespace after its last line
// break becomes depth tabs; at depth -1, the end of the file, it is
// kept as written. Every other run is left as written.
func (e *emitter) reindent(g gap, depth int) {
	if g.text == nil {
		return
	}
	run := g.text.Text[g.start:g.end]
	last := afterLastBreak(run)
	if last < 0 {
		return
	}
	var kept string
	switch g.class {
	case runTrimmed, runCollapse:
		kept = lineBreaks(run[:last], 2)
	case runPreserveBreaks:
		kept = lineBreaks(run[:last], -1)
	default:
		return
	}
	_, indent, _ := e.indents(g, depth)
	start := int(g.text.Start()-e.base) + g.start
	end := start + len(run)
	if start < 0 || end > len(e.src) || e.src[start:end] != string(run) {
		return // the text is not the source's: leave it
	}
	if text := kept + indent; text != string(run) {
		e.edits = append(e.edits, edit{start: start, end: end, text: text})
	}
}

// indents returns the indentation run g gives the line it starts, as
// written (old) and as reindent writes it at depth (indent), and whether
// reindent may change it: g holds a line break, its class lets the
// formatter change it, and its text is the source's.
func (e *emitter) indents(g gap, depth int) (old, indent string, ok bool) {
	if g.text == nil {
		return "", "", false
	}
	run := g.text.Text[g.start:g.end]
	last := afterLastBreak(run)
	if last < 0 {
		return "", "", false
	}
	old = string(run[last:])
	switch g.class {
	case runTrimmed, runCollapse, runPreserveBreaks:
	default:
		return old, old, false
	}
	start := int(g.text.Start()-e.base) + g.start
	if start < 0 || start+len(run) > len(e.src) || e.src[start:start+len(run)] != string(run) {
		return old, old, false
	}
	if depth < 0 {
		return old, old, true
	}
	return old, strings.Repeat("\t", depth), true
}

// startTag reindents the continuation lines of el's start tag (spec §3
// item 8): the whitespace after the last line break of a run between
// attributes becomes depth+1 tabs. When the source indents that line
// with tabs then spaces, the spaces are kept after the tabs (alignment).
// A > or /> on a line of its own goes at the tag's depth, without
// alignment. Runs without a line break, attribute values and the slash
// are as written. A start tag that is not a list of attributes and
// actions separated by whitespace is as written.
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
			align = alignment(run[last:])
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

// alignment returns the spaces of indent, a continuation line's
// indentation, when it is one or more tabs followed by one or more
// spaces, and "" otherwise.
func alignment(indent string) string {
	tabs := strings.TrimLeft(indent, "\t")
	if len(tabs) == len(indent) || strings.Trim(tabs, " ") != "" {
		return ""
	}
	return tabs
}

// lineBreaks returns the line breaks of run, in order and as written:
// CR LF, LF or CR. Its other bytes are dropped. When limit is not
// negative, only the first limit breaks are kept.
func lineBreaks(run []byte, limit int) string {
	var kept strings.Builder
	for i, n := 0, 0; i < len(run) && n != limit; i++ {
		switch {
		case run[i] == '\r' && i+1 < len(run) && run[i+1] == '\n':
			kept.WriteString("\r\n")
			i++
		case run[i] == '\n' || run[i] == '\r':
			kept.WriteByte(run[i])
		default:
			continue
		}
		n++
	}
	return kept.String()
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

// reachesPast reports whether a start tag that implies the end of open
// (impliedEnd) still does from inside an unclosed element named passed:
// the HTML parser's loops look past it to reach open (§13.2.6.4.7). An
// li, dt or dd is reached past any element but a special one other than
// address, div and p; a p past any but a button-scope barrier; a table
// part past any but table, template and html (table scope). An option is
// ended only when it is the current node, so never.
func reachesPast(open, passed string) bool {
	switch open {
	case "li", "dt", "dd":
		return !specialTags[passed] || passed == "address" || passed == "div" || passed == "p"
	case "p":
		return !buttonScopeBarriers[passed]
	case "option":
		return false
	}
	return passed != "table" && passed != "template" && passed != "html"
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
