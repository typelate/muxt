// Package format pretty-prints Go HTML template source for muxt fmt. It
// formats the syntax tree produced by muxt's own template parser, keeping
// HTML structure and template actions in a consistent layout.
package format

import (
	"slices"
	"strconv"
	"strings"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// Option configures the formatter.
type Option func(*formatter)

// Delimiters sets the left and right template delimiters.
// Defaults to "{{" and "}}".
func Delimiters(left, right string) Option {
	return func(f *formatter) {
		f.leftDelim = left
		f.rightDelim = right
	}
}

// Styles sets the author CSS the formatter classifies whitespace with:
// stylesheet files the templates use, templates whose <style> elements
// apply, and whether that is all the author CSS (see
// analysis.WhitespaceOptions). An element's display and white-space then
// come from that CSS: an element the CSS makes block-level may be laid
// out as a block, and one it makes preserve whitespace is written as it
// is in the source. Without Styles, or without AuthorCSSKnown, an element
// with a class or id is unknown and keeps its whitespace as written.
func Styles(options analysis.WhitespaceOptions) Option {
	return func(f *formatter) {
		f.whitespace = options
	}
}

// Node formats the given AST node as pretty-printed HTML template source.
// Indentation is one tab per level. Like gofmt, Node keeps the author's
// line structure: children are laid out one per line only where the
// source already breaks a gap the browser drops, and an open tag's
// attributes only where the source breaks them; no line width is
// measured. Node sees only the author CSS that Styles gives and the
// tree's own <style> elements, so without Styles an element with a class
// or id is unknown (decision 1) and keeps its whitespace as written. A
// tree holding markup the template parser read as text but a browser
// reads as a tag is written as it is (see hasUnparsedMarkup).
func Node(root template.Node, opts ...Option) string {
	f := &formatter{
		sb:          &strings.Builder{},
		atLineStart: true,
		leftDelim:   "{{",
		rightDelim:  "}}",
	}
	for _, opt := range opts {
		opt(f)
	}
	list, ok := root.(*template.ListNode)
	if !ok {
		list = &template.ListNode{Nodes: []template.Node{root}}
	}
	if hasUnparsedMarkup(list.Nodes) {
		for _, n := range list.Nodes {
			f.writeString(f.sourceText(n))
		}
		return f.sb.String()
	}
	f.styles = analysis.ComputeWhitespace([]*template.Tree{{Root: list}}, f.whitespace)
	f.root = list
	unsureMisnested(list, f.styles)
	unsureReopened(list, f.styles)
	unsureParsedElsewhere(list, f.styles)
	f.cls = classify(list, f.styles)
	f.opaque = make(map[template.Node]bool, len(f.cls.opaque))
	for _, n := range f.cls.opaque {
		f.opaque[n] = true
	}
	f.gapRuns = make(map[runKey]bool, len(f.cls.gaps))
	for _, g := range f.cls.gaps {
		if g.text != nil {
			f.gapRuns[runKey{g.text, g.start}] = true
		}
	}
	items, _ := splitChildren(list.Nodes)
	f.children(list, list.Nodes, listOptions{layout: f.layoutOf(list, items), root: true})
	return f.cleanUnnecessaryTrim(f.sb.String())
}

type formatter struct {
	sb          *strings.Builder
	indent      int // tabs writeIndent writes
	lineIndent  int // tabs at the start of the current line
	atLineStart bool
	leftDelim   string
	rightDelim  string
	whitespace  analysis.WhitespaceOptions

	root    *template.ListNode
	styles  analysis.Whitespaces
	cls     classes
	opaque  map[template.Node]bool // subtrees emitted from source
	gapRuns map[runKey]bool        // runs of text items emitted as gaps

	// continuation is the extra indentation of a line break kept in
	// inline layout: one inside an element or action, none at the root
	// or in block layout, where the items already sit one level deeper.
	continuation int

	// Output offsets of the trim markers the printer wrote: where "- "
	// follows a left delimiter and where " -" precedes a right one.
	leftTrims, rightTrims []int
}

// runKey names the run starting at byte start of text.
type runKey struct {
	text  *template.TextNode
	start int
}

// layoutKind is how a child list is printed: on the lines of its parent
// (inline) or one level deeper with the closing tag on its own line
// (block).
type layoutKind uint8

const (
	inlineLayout layoutKind = iota
	blockLayout
)

func (f *formatter) writeIndent() {
	for range f.indent {
		f.sb.WriteByte('\t')
	}
	f.lineIndent = f.indent
	f.atLineStart = false
}

func (f *formatter) newline() {
	f.sb.WriteByte('\n')
	f.atLineStart = true
}

func (f *formatter) writeString(s string) {
	f.sb.WriteString(s)
	f.atLineStart = false
}

// writeLeftDelim writes the left delimiter and, with trim, the "- "
// marker after it.
func (f *formatter) writeLeftDelim(trim bool) {
	f.writeString(f.leftDelim)
	if trim {
		f.leftTrims = append(f.leftTrims, f.sb.Len())
		f.writeString("- ")
	}
}

// writeRightDelim writes, with trim, the " -" marker and then the right
// delimiter.
func (f *formatter) writeRightDelim(trim bool) {
	if trim {
		f.rightTrims = append(f.rightTrims, f.sb.Len())
		f.writeString(" -")
	}
	f.writeString(f.rightDelim)
}

// writeActionWithTrim writes a template action with optional trim markers.
func (f *formatter) writeActionWithTrim(content string, trimLeft, trimRight bool) {
	f.writeLeftDelim(trimLeft)
	f.writeString(content)
	f.writeRightDelim(trimRight)
}

// writeRun writes the whitespace run g as its class allows (spec §6.3):
// a preserved run byte for byte; a preserve-breaks run as its line
// breaks alone, or as written when it has none. In block layout a run
// that takes a break becomes one newline plus the current indentation,
// or a blank line when it is reflowable and held a blank line. Any other
// run is written as collapse-only: empty stays empty, a trimmed run
// becomes one space beside its marker, spaces and tabs become one space,
// and a run with a line break becomes one newline plus the current
// indentation and the continuation indent. The run stays in the node it
// was written in: no whitespace moves across a tag or an action. The
// caller handles the runs at a list boundary.
func (f *formatter) writeRun(g gap, layout layoutKind) {
	run := g.bytes()
	switch {
	case g.class == runPreserved:
		f.writeString(run)
	case g.class == runPreserveBreaks:
		n := lineBreaks(run)
		if n == 0 {
			f.writeString(run)
			return
		}
		for range n {
			f.newline()
		}
	case layout == blockLayout && takesBreak(g):
		f.newline()
		if g.class == runReflowable && lineBreaks(run) > 1 {
			f.newline()
		}
		f.writeIndent()
	case run == "":
	case g.class == runTrimmed, lineBreaks(run) == 0:
		f.writeString(" ")
	default:
		f.newline()
		f.indent += f.continuation
		f.writeIndent()
		f.indent -= f.continuation
	}
}

// takesBreak reports whether a newline and indentation may replace the
// run g: it is reflowable or trimmed, so it never renders, or it is a
// collapse-only run that already holds a line break.
func takesBreak(g gap) bool {
	switch g.class {
	case runReflowable, runTrimmed:
		return true
	case runCollapseOnly:
		return lineBreaks(g.bytes()) > 0
	}
	return false
}

// blockLayoutAllowed reports whether the items of list may be laid out
// one per line (spec §6.4): every gap, the two boundary gaps included,
// takes a break. The tree root's boundary gaps are the start and end of
// the file, where block layout writes no break, so they do not count.
func (f *formatter) blockLayoutAllowed(list template.Node, items []template.Node) bool {
	for i := 0; i <= len(items); i++ {
		if list == f.root && (i == 0 || i == len(items)) {
			continue
		}
		if !takesBreak(f.cls.gaps[gapKey{list, i}]) {
			return false
		}
	}
	return true
}

// blockLayoutPreferred reports whether the source already lays the
// items of list out as a block: an item is block-level, or a gap the
// browser drops held a line break in the source. The formatter keeps the
// author's line structure (spec amendment, gofmt model): no width is
// measured, so a long line stays long. A line break in a collapse-only
// gap renders as a space, so it keeps its line in inline layout instead
// of making the list a block.
func (f *formatter) blockLayoutPreferred(list template.Node, items []template.Node) bool {
	for i := 0; i <= len(items); i++ {
		g := f.cls.gaps[gapKey{list, i}]
		if g.class != runCollapseOnly && lineBreaks(g.bytes()) > 0 {
			return true
		}
	}
	return slices.ContainsFunc(items, f.isBlockItem)
}

// layoutOf chooses the layout of list: block when it is allowed and
// preferred, else inline.
func (f *formatter) layoutOf(list template.Node, items []template.Node) layoutKind {
	if f.blockLayoutAllowed(list, items) && f.blockLayoutPreferred(list, items) {
		return blockLayout
	}
	return inlineLayout
}

// isBlockItem reports whether n is a block-level or table-internal
// element, or control flow whose arms hold one.
func (f *formatter) isBlockItem(n template.Node) bool {
	switch n := n.(type) {
	case *template.HTMLElementNode:
		ws, ok := f.styles[n]
		return ok && (ws.Display == analysis.DisplayBlock || ws.Display == analysis.DisplayTableInternal)
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		b := branchOf(n)
		return f.listHasBlockItem(b.List) || f.listHasBlockItem(b.ElseList)
	}
	return false
}

func (f *formatter) listHasBlockItem(l *template.ListNode) bool {
	if l == nil {
		return false
	}
	items, _ := splitChildren(l.Nodes)
	return slices.ContainsFunc(items, f.isBlockItem)
}

// bytes returns the run's text; it is empty for an empty gap.
func (g gap) bytes() string {
	if g.text == nil {
		return ""
	}
	return string(g.text.Text[g.start:g.end])
}

// lineBreaks counts the line breaks in a run: LF, CR, and CR LF each
// count once, as the HTML parser normalizes them.
func lineBreaks(run string) int {
	n := 0
	for i := 0; i < len(run); i++ {
		crlf := run[i] == '\r' && i+1 < len(run) && run[i+1] == '\n'
		if (run[i] == '\n' || run[i] == '\r') && !crlf {
			n++
		}
	}
	return n
}

// listOptions say how children prints one child list.
type listOptions struct {
	layout layoutKind
	// outer is the indentation of the line the list closes on.
	outer int
	// root marks the tree root: nothing closes it.
	root bool
	// trimStart and trimEnd report that the formatter writes a trim
	// marker next to the start or the end boundary, so that boundary's
	// run never renders.
	trimStart, trimEnd bool
}

// children prints the items of the list key (its children are nodes)
// with the gaps between them. The current indentation is the items'
// level. In block layout every gap becomes a newline and indentation
// (spec §6.4); in inline layout a reflowable boundary run is dropped and
// every other gap is written as collapse-only. children leaves the
// cursor where the closing tag or action goes.
func (f *formatter) children(key template.Node, nodes []template.Node, o listOptions) {
	items, _ := splitChildren(nodes)
	inner := f.indent
	defer func(c int) { f.continuation = c }(f.continuation)
	f.continuation = 0
	if o.layout == inlineLayout && !o.root {
		f.continuation = 1
	}
	for i := 0; i <= len(items); i++ {
		g := f.cls.gaps[gapKey{key, i}]
		switch {
		case i == len(items):
			f.indent = o.outer
			f.closeGap(g, o)
			f.indent = inner
			continue
		case i == 0:
			f.openGap(g, o)
		default:
			f.betweenGap(items[i-1], items[i], g, o.layout)
		}
		f.item(items[i], o.layout)
	}
}

// droppedInline reports whether inline layout leaves a boundary run out:
// it is reflowable, or trim markers the formatter writes remove it. A
// trimmed run is not left out, since inline layout writes it as one
// space beside its marker. Block layout replaces these runs, and
// trimmed ones, with a break.
func droppedInline(g gap, trimmed bool) bool {
	return trimmed || g.class == runReflowable
}

func (f *formatter) openGap(g gap, o listOptions) {
	switch {
	case o.layout == blockLayout && (droppedInline(g, o.trimStart) || g.class == runTrimmed):
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
	case o.layout == inlineLayout && droppedInline(g, o.trimStart):
	default:
		f.writeRun(g, o.layout)
	}
}

func (f *formatter) closeGap(g gap, o listOptions) {
	switch {
	case o.layout == blockLayout && (droppedInline(g, o.trimEnd) || g.class == runTrimmed):
		if o.root {
			return
		}
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
	case o.layout == inlineLayout && droppedInline(g, o.trimEnd):
	default:
		f.writeRun(g, o.layout)
	}
}

func (f *formatter) betweenGap(prev, next template.Node, g gap, layout layoutKind) {
	_, prevDefine := prev.(*template.DefineNode)
	_, nextDefine := next.(*template.DefineNode)
	if prevDefine && nextDefine && g.class != runPreserved && (lineBreaks(g.bytes()) > 0 || layout == blockLayout) {
		// A blank line between adjacent defines. Block layout puts a
		// break in every gap, so it adds the blank line at once.
		f.newline()
		f.newline()
		f.writeIndent()
		return
	}
	f.writeRun(g, layout)
}

func (f *formatter) item(n template.Node, layout layoutKind) {
	if text, ok := n.(*template.TextNode); ok {
		f.text(text, layout)
		return
	}
	f.node(n)
}

// text writes a text item: content bytes verbatim, each run of its own
// by its class. Its leading and trailing runs, when they are the gaps
// next to it, are written by children.
func (f *formatter) text(n *template.TextNode, layout layoutKind) {
	runs := f.cls.runs[n]
	for _, s := range splitRuns(n.Text) {
		raw := string(n.Text[s.start:s.end])
		switch {
		case !s.space:
			f.writeString(raw)
		case f.gapRuns[runKey{n, s.start}]:
		case len(runs) > 0 && runs[0].start == s.start:
			f.writeRun(runs[0], layout)
			runs = runs[1:]
		default:
			f.writeString(raw) // not classified: as written
		}
	}
}

func (f *formatter) node(n template.Node) {
	if n == nil {
		return
	}
	switch n := n.(type) {
	case *template.ListNode:
		f.listNode(n)
	case *template.HTMLDocumentFragmentNode:
		f.documentFragment(n)
	case *template.TextNode:
		f.text(n, blockLayout)
	case *template.HTMLElementNode:
		f.htmlElement(n)
	case *template.HTMLAttributeNode:
		f.htmlAttribute(n)
	case *template.HTMLCommentNode:
		f.htmlComment(n)
	case *template.HTMLDoctypeNode:
		f.htmlDoctype(n)
	case *template.ActionNode:
		f.actionNode(n)
	case *template.TemplateNode:
		f.templateNode(n)
	case *template.IfNode:
		f.branchNode(&n.BranchNode, "if")
	case *template.RangeNode:
		f.branchNode(&n.BranchNode, "range")
	case *template.WithNode:
		f.branchNode(&n.BranchNode, "with")
	case *template.DefineNode:
		f.defineNode(n)
	case *template.BlockNode:
		f.blockNode(n)
	case *template.CommentNode:
		f.commentNode(n)
	case *template.BreakNode:
		f.writeActionWithTrim("break", n.TrimLeft, n.TrimRight)
	case *template.ContinueNode:
		f.writeActionWithTrim("continue", n.TrimLeft, n.TrimRight)
	}
}

// listNode and documentFragment print lists that children does not
// reach, which a parsed tree does not have; they write each node in turn.
func (f *formatter) listNode(n *template.ListNode) {
	if n == nil {
		return
	}
	for _, child := range n.Nodes {
		f.node(child)
	}
}

func (f *formatter) documentFragment(n *template.HTMLDocumentFragmentNode) {
	for _, child := range n.Children {
		f.node(child)
	}
}

// writeSource writes el's children as written and its closing tag.
func (f *formatter) writeSource(el *template.HTMLElementNode) {
	for _, child := range el.Children {
		f.writeString(f.sourceText(child))
	}
	f.closeTag(el)
}

// voidElements are the HTML elements with no content, whose start tag
// the formatter writes without a slash (as the template parser's list).
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

// sourceText is n as written. An unclosed element's String is its start
// tag alone, so its children are added after it. A comment action's
// String is its text alone, so its delimiters and trim markers are added
// around it (text/template allows nothing else between them).
func (f *formatter) sourceText(n template.Node) string {
	if c, ok := n.(*template.CommentNode); ok {
		var b strings.Builder
		b.WriteString(f.leftDelim)
		if c.TrimLeft {
			b.WriteString("- ")
		}
		b.WriteString(c.Text)
		if c.TrimRight {
			b.WriteString(" -")
		}
		b.WriteString(f.rightDelim)
		return b.String()
	}
	el, ok := n.(*template.HTMLElementNode)
	if !ok || el.Closed || el.Void {
		return n.String()
	}
	var b strings.Builder
	b.WriteString(el.String())
	for _, child := range el.Children {
		b.WriteString(f.sourceText(child))
	}
	return b.String()
}

// closeTag writes el's closing tag when the source has one. An element
// the source leaves unclosed stays unclosed: the browser may close it
// earlier than the template tree does (a p before a div), and there an
// added end tag is content of its own (a stray </p> is an empty p).
func (f *formatter) closeTag(el *template.HTMLElementNode) {
	if !el.Closed {
		return
	}
	f.writeString("</")
	f.writeString(el.TagName)
	f.writeString(">")
}

func (f *formatter) htmlElement(n *template.HTMLElementNode) {
	// Open tag (positioning handled by children)
	f.writeString("<")
	f.writeString(n.TagName)
	f.openTagRest(n)
	if n.Void {
		return
	}

	if f.opaque[n] {
		f.writeSource(n)
		return
	}

	if len(n.Children) == 0 {
		// Empty element: closing tag on same line
		f.closeTag(n)
		return
	}

	items, _ := splitChildren(n.Children)
	layout := f.layoutOf(n, items)
	outer := f.indent
	if layout == blockLayout {
		f.indent++
	}
	f.children(n, n.Children, listOptions{layout: layout, outer: outer})
	f.indent = outer
	f.closeTag(n)
}

// openTagRest writes the open tag of n after its name: its attributes
// and the closing >. Attribute wrapping follows the source (spec
// amendment, gofmt model): each run of whitespace between attributes
// becomes one space, or, when it holds a line break, a line break and
// the tag's indentation plus one tab; no width is measured. Each
// attribute is written as in the source, its quoting and any line
// breaks in its value kept. A > the source puts on a line of its own
// stays there, at the tag's indentation. An open tag the template
// parser does not split into whitespace-separated attributes and
// actions (control flow, comments, an end action, text between them) is
// written from source.
func (f *formatter) openTagRest(n *template.HTMLElementNode) {
	region, attrs, tail, ok := splitOpenTag(n)
	if !ok {
		f.writeString(region)
		return
	}
	tagIndent := f.lineIndent
	for _, attr := range attrs {
		f.tagSpace(attr.before, tagIndent+1)
		f.writeString(attr.text)
	}
	before, after, slash := strings.Cut(tail, "/")
	if slash && !voidElements[strings.ToLower(n.TagName)] {
		// <a/> as written: on an element that is not void the browser
		// ignores the slash and the content that follows is the
		// element's, so a plain > would read differently next pass.
		f.tagSpace(before, tagIndent)
		f.writeString("/")
		f.tagSpace(after, tagIndent)
		f.writeString(">")
		return
	}
	// A void element's slash means nothing and is dropped.
	if lineBreaks(tail) > 0 {
		f.tagSpace("\n", tagIndent)
	}
	f.writeString(">")
}

// tagSpace writes a run of whitespace inside an open tag: nothing for an
// empty run, one space for a run without a line break, else a line break
// and indent tabs.
func (f *formatter) tagSpace(ws string, indent int) {
	switch {
	case ws == "":
	case lineBreaks(ws) == 0:
		f.writeString(" ")
	default:
		f.newline()
		outer := f.indent
		f.indent = indent
		f.writeIndent()
		f.indent = outer
	}
}

// openTagAttr is one attribute or action of an open tag as written, and
// the whitespace before it.
type openTagAttr struct{ before, text string }

// splitOpenTag splits the open tag of n, after its name, into its
// attributes, each with the whitespace before it, and the tail between
// the last one and the closing >. region is the source of that part of
// the tag, > included. ok is false when the tag is not a list of
// attributes and actions with only whitespace between them and a tail
// of whitespace and at most one slash: the caller then writes region as
// it is.
func splitOpenTag(n *template.HTMLElementNode) (region string, attrs []openTagAttr, tail string, ok bool) {
	src := n.String()
	start, attrStart, attrEnd := int(n.Start()), int(n.AttrStart), int(n.AttrEnd)
	if attrStart < start || attrEnd > start+len(src) || attrEnd <= attrStart {
		return "", nil, "", false
	}
	region = src[attrStart-start : attrEnd-start]
	if !strings.HasSuffix(region, ">") {
		return region, nil, "", false
	}
	prev := attrStart
	for _, attr := range n.Attr {
		switch attr.(type) {
		case *template.HTMLAttributeNode, *template.ActionNode, *template.TemplateNode:
		default:
			return region, nil, "", false
		}
		at, end := int(attr.Start()), int(attr.End())
		if at < prev || end < at || end > attrEnd-1 {
			return region, nil, "", false
		}
		before := src[prev-start : at-start]
		if strings.Trim(before, tagWhitespace) != "" || before == "" && prev == attrStart {
			return region, nil, "", false
		}
		attrs = append(attrs, openTagAttr{before: before, text: src[at-start : end-start]})
		prev = end
	}
	tail = src[prev-start : attrEnd-start-1]
	if strings.Count(tail, "/") > 1 || strings.Trim(tail, tagWhitespace+"/") != "" {
		return region, nil, "", false
	}
	return region, attrs, tail, true
}

// tagWhitespace is the whitespace the HTML tokenizer skips between
// attributes.
const tagWhitespace = " \t\n\r\f"

func (f *formatter) htmlAttribute(n *template.HTMLAttributeNode) {
	if n.Namespace != "" {
		f.writeString(n.Namespace)
		f.sb.WriteByte(':')
	}
	for _, k := range n.Key {
		f.writeString(k.String())
	}
	if n.HasValue {
		f.sb.WriteByte('=')
		for _, v := range n.Value {
			f.writeString(v.String())
		}
	}
}

func (f *formatter) htmlComment(n *template.HTMLCommentNode) {
	f.writeString("<!--")
	for _, child := range n.Children {
		f.writeString(child.String())
	}
	f.writeString("-->")
}

func (f *formatter) htmlDoctype(n *template.HTMLDoctypeNode) {
	f.writeString(n.Text)
}

func (f *formatter) actionNode(n *template.ActionNode) {
	f.writeLeftDelim(n.TrimLeft)
	f.writePipe(n.Pipe)
	f.writeRightDelim(n.TrimRight)
}

func (f *formatter) templateNode(n *template.TemplateNode) {
	f.templateCall(n, n.TrimLeft, n.TrimRight)
}

func (f *formatter) templateCall(n *template.TemplateNode, trimLeft, trimRight bool) {
	f.writeLeftDelim(trimLeft)
	f.writeString("template ")
	f.writeString(strconv.Quote(n.Name))
	if n.Pipe != nil {
		f.sb.WriteByte(' ')
		f.writePipe(n.Pipe)
	}
	f.writeRightDelim(trimRight)
}

// trimPipeDelim strips a trailing right delimiter (and optional trim marker)
// from the pipe's source text. PipeNode.String() returns raw source whose
// End() includes the closing delimiter, so we must trim it.
func trimPipeDelim(s, rightDelim string) string {
	s = strings.TrimSuffix(s, rightDelim)
	s = strings.TrimRight(s, " \t")
	s = strings.TrimSuffix(s, "-")
	s = strings.TrimRight(s, " \t")
	return s
}

func (f *formatter) branchNode(n *template.BranchNode, keyword string) {
	f.writeActionWithTrim(keyword+" "+f.pipeText(n.Pipe), n.StartTrimLeft, n.StartTrimRight)
	f.body(n.List, n.StartTrimRight)
	endTrimLeft := n.EndTrimLeft
	if n.ElseList != nil {
		endTrimLeft = f.formatElseList(n, n.ElseList)
	}
	f.writeActionWithTrim("end", endTrimLeft, n.EndTrimRight)
}

// body prints a branch body or else list in the layout its gaps choose:
// inline on the branch's line, or as a block one level deeper (at the
// same level after a -}}).
func (f *formatter) body(list *template.ListNode, trimRight bool) {
	if list == nil || len(list.Nodes) == 0 {
		return
	}
	outer := f.indent
	items, _ := splitChildren(list.Nodes)
	if f.layoutOf(list, items) == inlineLayout {
		f.children(list, list.Nodes, listOptions{layout: inlineLayout, outer: outer})
		return
	}
	if !trimRight {
		f.indent++
	}
	f.children(list, list.Nodes, listOptions{layout: blockLayout, outer: outer})
	f.indent = outer
}

// formatElseList handles the else portion of a branch, recursing for
// else-if chains. It returns the left trim of the {{end}} that closes
// the chain. A merged "{{else if ...}}" takes its left trim from the
// {{else}}, which follows the same body, and its right trim from the
// inner branch, which precedes the same body; the {{end}} takes its left
// trim from the innermost branch, whose last body it follows. The trims
// that only removed the gaps around the inner branch go with them.
func (f *formatter) formatElseList(parent *template.BranchNode, elseList *template.ListNode) (endTrimLeft bool) {
	if chain, chainKeyword := f.elseChain(elseList); chain != nil {
		f.writeActionWithTrim("else "+chainKeyword+" "+f.pipeText(chain.Pipe), parent.ElseTrimLeft, chain.StartTrimRight)
		f.body(chain.List, chain.StartTrimRight)
		if chain.ElseList != nil {
			return f.formatElseList(chain, chain.ElseList)
		}
		return chain.EndTrimLeft
	}
	f.writeActionWithTrim("else", parent.ElseTrimLeft, parent.ElseTrimRight)
	f.body(elseList, parent.ElseTrimRight)
	return parent.EndTrimLeft
}

func (f *formatter) defineNode(n *template.DefineNode) {
	f.templateBody("define "+strconv.Quote(n.Name), n.Body, n.StartTrimLeft, n.StartTrimRight, n.EndTrimLeft, n.EndTrimRight)
}

func (f *formatter) blockNode(n *template.BlockNode) {
	f.templateBody("block "+strconv.Quote(n.Name)+" "+f.pipeText(n.Pipe), n.Body, n.StartTrimLeft, n.StartTrimRight, n.EndTrimLeft, n.EndTrimRight)
}

// templateBody prints a define or block: its start action, its body and
// its end action, each action with the trim markers the source wrote
// (spec §6.5). The body's caller is unknown, so an edge run is
// reflowable only when the body's first or last item is block-level. The
// body is laid out as a block when every gap takes a break, or is an
// empty collapse-only edge gap, and block layout is preferred. Into an
// empty collapse-only edge gap block layout inserts a break, and the
// formatter adds a trim marker that removes exactly that break, so the
// body renders as before. No trim marker is added that would remove
// whitespace written in the source.
func (f *formatter) templateBody(start string, body *template.ListNode, startTrimLeft, startTrimRight, endTrimLeft, endTrimRight bool) {
	if body == nil || len(body.Nodes) == 0 {
		f.writeActionWithTrim(start, startTrimLeft, startTrimRight)
		f.writeActionWithTrim("end", endTrimLeft, endTrimRight)
		return
	}
	outer := f.indent
	layout, trimStart, trimEnd := f.bodyLayout(body)
	f.writeActionWithTrim(start, startTrimLeft, startTrimRight || trimStart)
	if layout == blockLayout {
		f.indent++
	}
	f.children(body, body.Nodes, listOptions{layout: layout, outer: outer, trimStart: trimStart, trimEnd: trimEnd})
	f.indent = outer
	f.writeActionWithTrim("end", endTrimLeft || trimEnd, endTrimRight)
}

// bodyLayout chooses the layout of a define or block body and reports
// whether the formatter adds a trim marker at its start and its end to
// remove the break block layout inserts into an empty collapse-only edge
// gap. A line break in a collapse-only edge gap prefers block layout: it
// keeps the author's line structure, and block layout writes it as one
// line break, which renders as the same space.
func (f *formatter) bodyLayout(body *template.ListNode) (layout layoutKind, trimStart, trimEnd bool) {
	items, _ := splitChildren(body.Nodes)
	emptyCollapseOnly := func(i int) bool {
		g := f.cls.gaps[gapKey{body, i}]
		return g.class == runCollapseOnly && g.bytes() == ""
	}
	preferred := f.blockLayoutPreferred(body, items)
	for i := 0; i <= len(items); i++ {
		edge := i == 0 || i == len(items)
		g := f.cls.gaps[gapKey{body, i}]
		switch {
		case edge && emptyCollapseOnly(i):
		case !takesBreak(g):
			return inlineLayout, false, false
		case edge && lineBreaks(g.bytes()) > 0:
			preferred = true
		}
	}
	if !preferred {
		return inlineLayout, false, false
	}
	return blockLayout, emptyCollapseOnly(0), emptyCollapseOnly(len(items))
}

func (f *formatter) commentNode(n *template.CommentNode) {
	f.writeLeftDelim(n.TrimLeft)
	f.writeString(n.Text)
	f.writeRightDelim(n.TrimRight)
}

func (f *formatter) writePipe(p *template.PipeNode) {
	if p == nil {
		return
	}
	f.writeString(trimPipeDelim(p.String(), f.rightDelim))
}

// pipeText returns the pipe content as a string with delimiters stripped.
func (f *formatter) pipeText(p *template.PipeNode) string {
	if p == nil {
		return ""
	}
	return trimPipeDelim(p.String(), f.rightDelim)
}

// elseChain detects when an else list holds a single branch (IfNode or
// WithNode) and returns it for "else if" / "else with" chaining. Gaps
// around the branch may be empty or reflowable, since the merged form
// writes no whitespace there.
func (f *formatter) elseChain(elseList *template.ListNode) (*template.BranchNode, string) {
	if elseList == nil {
		return nil, ""
	}
	items, _ := splitChildren(elseList.Nodes)
	if len(items) != 1 {
		return nil, ""
	}
	for i := range 2 {
		if g := f.cls.gaps[gapKey{elseList, i}]; g.text != nil && g.class != runReflowable {
			return nil, ""
		}
	}
	switch n := items[0].(type) {
	case *template.IfNode:
		return &n.BranchNode, "if"
	case *template.WithNode:
		return &n.BranchNode, "with"
	}
	return nil, ""
}

// cleanUnnecessaryTrim removes the trim markers the printer wrote where
// the run they would trim is empty in the output: a left marker that
// follows content or starts the output, and a right marker that precedes
// content. A right marker at the end of the output stays, because Source
// appends the file's final run after it. Markers inside text emitted
// from source are not touched.
func (f *formatter) cleanUnnecessaryTrim(s string) string {
	drop := make(map[int]int) // output offset → bytes to drop
	for _, at := range f.leftTrims {
		before := at - len(f.leftDelim) - 1
		if before < 0 || !isRunByte(s[before]) {
			drop[at] = len("- ")
		}
	}
	for _, at := range f.rightTrims {
		after := at + len(" -") + len(f.rightDelim)
		if after < len(s) && !isRunByte(s[after]) {
			drop[at] = len(" -")
		}
	}
	if len(drop) == 0 {
		return s
	}
	offsets := make([]int, 0, len(drop))
	for at := range drop {
		offsets = append(offsets, at)
	}
	slices.Sort(offsets)
	var b strings.Builder
	b.Grow(len(s))
	prev := 0
	for _, at := range offsets {
		b.WriteString(s[prev:at])
		prev = at + drop[at]
	}
	b.WriteString(s[prev:])
	return b.String()
}

// Source formats a Go HTML template file as Node does, with the same
// options: it reads the author CSS Styles gives and the file's <style>
// elements. The file's final whitespace run is left exactly as written:
// Source never adds or removes a trailing newline. A CR LF outside an
// opaque region is written as LF, as the HTML parser reads it; inside
// one (pre, textarea, script, style, ...) the bytes are kept.
// A file with no HTML element is not a page (a plain-text email, a
// text-only partial), so its whitespace is all preserved and Source
// returns it unchanged.
func Source(filename, src string, opts ...Option) (string, error) {
	f := &formatter{leftDelim: "{{", rightDelim: "}}"}
	for _, o := range opts {
		o(f)
	}
	tree, err := template.ParseTree(filename, filename, src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
		LeftDelim:   f.leftDelim,
		RightDelim:  f.rightDelim,
	})
	if err != nil {
		return "", err
	}
	if !hasElement(tree) {
		return src, nil
	}
	out := Node(tree.Root, opts...)
	return strings.TrimRight(out, whitespace) + trailingWhitespace(src), nil
}

// hasUnparsedMarkup reports whether text in nodes, outside raw text and
// RCDATA elements and comments, holds a < that the HTML tokenizer reads
// as the start of a tag, end tag, comment or processing instruction (<
// then a letter, /, ! or ?). The template parser keeps markup it cannot
// read as an element (<p <b>, </ b>, <?x?>) as text, so its tree and the
// browser's disagree there, and the whitespace around it cannot be
// classified.
func hasUnparsedMarkup(nodes []template.Node) bool {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.TextNode:
			if startsMarkup(n.Text) {
				return true
			}
		case *template.HTMLDocumentFragmentNode:
			if hasUnparsedMarkup(n.Children) {
				return true
			}
		case *template.HTMLElementNode:
			if !rawTextTags[strings.ToLower(n.TagName)] && hasUnparsedMarkup(n.Children) {
				return true
			}
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil && hasUnparsedMarkup(b.List.Nodes) || b.ElseList != nil && hasUnparsedMarkup(b.ElseList.Nodes) {
				return true
			}
		case *template.DefineNode:
			if n.Body != nil && hasUnparsedMarkup(n.Body.Nodes) {
				return true
			}
		case *template.BlockNode:
			if n.Body != nil && hasUnparsedMarkup(n.Body.Nodes) {
				return true
			}
		}
	}
	return false
}

func startsMarkup(text []byte) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		c := text[i+1]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '/' || c == '!' || c == '?' {
			return true
		}
	}
	return false
}

// rawTextTags name the elements whose content the HTML tokenizer reads
// as text up to their end tag (raw text and RCDATA, HTML §13.1.2), with
// scripting enabled.
var rawTextTags = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true,
}

func hasElement(tree *template.Tree) bool {
	found := false
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if _, ok := n.(*template.HTMLElementNode); ok {
			found = true
		}
	})
	return found
}

const whitespace = " \t\r\n"

func trailingWhitespace(s string) string {
	return s[len(strings.TrimRight(s, whitespace)):]
}
