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

// MaxLineWidth sets the maximum line width before the formatter prefers
// block layout. Defaults to 80.
func MaxLineWidth(n int) Option {
	return func(f *formatter) {
		f.maxLineWidth = n
	}
}

// MaxInlineAttributes sets the maximum number of attributes that can be
// kept on one line. Defaults to 3.
func MaxInlineAttributes(n int) Option {
	return func(f *formatter) {
		f.maxInlineAttrs = n
	}
}

// Node formats the given AST node as pretty-printed HTML template source.
func Node(root template.Node, opts ...Option) string {
	f := &formatter{
		sb:             &strings.Builder{},
		atLineStart:    true,
		leftDelim:      "{{",
		rightDelim:     "}}",
		maxLineWidth:   80,
		maxInlineAttrs: 3,
	}
	for _, opt := range opts {
		opt(f)
	}
	list, ok := root.(*template.ListNode)
	if !ok {
		list = &template.ListNode{Nodes: []template.Node{root}}
	}
	f.styles = analysis.ComputeWhitespace([]*template.Tree{{Root: list}}, analysis.WhitespaceOptions{})
	f.root = list
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
	f.children(list, list.Nodes, listOptions{layout: f.layoutOf(list, items, itemsWidth(items)), root: true})
	return f.cleanUnnecessaryTrim(f.sb.String())
}

type formatter struct {
	sb             *strings.Builder
	indent         int
	atLineStart    bool
	leftDelim      string
	rightDelim     string
	maxLineWidth   int
	maxInlineAttrs int

	root    *template.ListNode
	styles  analysis.Whitespaces
	cls     classes
	opaque  map[template.Node]bool // subtrees emitted from source
	gapRuns map[runKey]bool        // runs of text items emitted as gaps

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
// indentation. The caller handles the runs at a list boundary.
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
		f.writeIndent()
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

// blockLayoutPreferred reports whether block layout reads better for the
// items of list: an item is block-level, a gap held a line break in the
// source, or the one-line form, width columns at the current
// indentation, is wider than the maximum line width.
func (f *formatter) blockLayoutPreferred(list template.Node, items []template.Node, width int) bool {
	for i := 0; i <= len(items); i++ {
		if lineBreaks(f.cls.gaps[gapKey{list, i}].bytes()) > 0 {
			return true
		}
	}
	if slices.ContainsFunc(items, f.isBlockItem) {
		return true
	}
	return f.indent*tabColumns+width > f.maxLineWidth
}

// layoutOf chooses the layout of list: block when it is allowed and
// preferred, else inline.
func (f *formatter) layoutOf(list template.Node, items []template.Node, width int) layoutKind {
	if f.blockLayoutAllowed(list, items) && f.blockLayoutPreferred(list, items, width) {
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

// tabColumns is the width of one indentation tab (spec §6.6).
const tabColumns = 4

// contentWidth is the width of s on one line, not counting whitespace
// runs. Formatting only rewrites runs and drops bytes (spaces inside
// actions, trim markers, merged else actions), so a list never measures
// wider once formatted, and a list that fits stays inline.
func contentWidth(s string) int {
	n := 0
	for _, seg := range splitRuns([]byte(s)) {
		if !seg.space {
			n += seg.end - seg.start
		}
	}
	return n
}

// itemsWidth is the one-line width of items.
func itemsWidth(items []template.Node) int {
	n := 0
	for _, item := range items {
		n += contentWidth(item.String())
	}
	return n
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
	// trimEdges reports that the formatter writes trim markers next to
	// both boundaries, so the boundary runs never render.
	trimEdges bool
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

// droppable reports whether a boundary run never renders and may be
// left out: it is reflowable, or the formatter trims it.
func droppable(g gap, o listOptions) bool {
	return o.trimEdges || g.class == runReflowable
}

func (f *formatter) openGap(g gap, o listOptions) {
	switch {
	case o.layout == blockLayout && (droppable(g, o) || g.class == runTrimmed):
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
	case o.layout == inlineLayout && droppable(g, o):
	default:
		f.writeRun(g, o.layout)
	}
}

func (f *formatter) closeGap(g gap, o listOptions) {
	switch {
	case o.layout == blockLayout && (droppable(g, o) || g.class == runTrimmed):
		if o.root {
			return
		}
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
	case o.layout == inlineLayout && droppable(g, o):
	default:
		f.writeRun(g, o.layout)
	}
}

func (f *formatter) betweenGap(prev, next template.Node, g gap, layout layoutKind) {
	_, prevDefine := prev.(*template.DefineNode)
	_, nextDefine := next.(*template.DefineNode)
	if prevDefine && nextDefine && g.class != runPreserved && lineBreaks(g.bytes()) > 0 {
		// A blank line between adjacent defines.
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
		f.writeString(child.String())
	}
	f.writeString("</")
	f.writeString(el.TagName)
	f.writeString(">")
}

func (f *formatter) htmlElement(n *template.HTMLElementNode) {
	// Open tag (positioning handled by children)
	f.writeString("<")
	f.writeString(n.TagName)
	f.formatAttributes(n)
	f.writeString(">")
	if n.Void {
		return
	}

	if f.opaque[n] {
		f.writeSource(n)
		return
	}

	if len(n.Children) == 0 {
		// Empty element: closing tag on same line
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
		return
	}

	items, _ := splitChildren(n.Children)
	layout := f.layoutOf(n, items, estimateElementInlineLength(n))
	outer := f.indent
	if layout == blockLayout {
		f.indent++
	}
	f.children(n, n.Children, listOptions{layout: layout, outer: outer})
	f.indent = outer
	f.writeString("</")
	f.writeString(n.TagName)
	f.writeString(">")
}

func (f *formatter) formatAttributes(n *template.HTMLElementNode) {
	if len(n.Attr) == 0 {
		return
	}

	// Check for branch nodes in attribute position — emit original source
	hasBranchAttr := false
	for _, attr := range n.Attr {
		switch attr.(type) {
		case *template.IfNode, *template.RangeNode, *template.WithNode:
			hasBranchAttr = true
		}
	}

	if hasBranchAttr {
		for _, attr := range n.Attr {
			f.sb.WriteByte(' ')
			f.writeString(attr.String())
		}
		return
	}

	// Count attributes and measure total length
	totalLen := 0
	attrCount := 0
	for _, attr := range n.Attr {
		attrCount++
		totalLen += len(attr.String())
	}

	multiLine := attrCount > f.maxInlineAttrs || totalLen > f.maxLineWidth

	if multiLine {
		f.indent++
		for _, attr := range n.Attr {
			f.newline()
			f.writeIndent()
			f.node(attr)
		}
		f.indent--
	} else {
		for _, attr := range n.Attr {
			f.sb.WriteByte(' ')
			f.node(attr)
		}
	}
}

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
	if n.ElseList != nil {
		f.formatElseList(n, n.ElseList)
	}
	f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
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
	if f.layoutOf(list, items, itemsWidth(items)) == inlineLayout {
		f.children(list, list.Nodes, listOptions{layout: inlineLayout, outer: outer})
		return
	}
	if !trimRight {
		f.indent++
	}
	f.children(list, list.Nodes, listOptions{layout: blockLayout, outer: outer})
	f.indent = outer
}

// formatElseList handles the else portion of a branch, recursing for else-if chains.
func (f *formatter) formatElseList(parent *template.BranchNode, elseList *template.ListNode) {
	if chain, chainKeyword := f.elseChain(elseList); chain != nil {
		// For else-if/else-with chains, the inner BranchNode carries the trim
		// for the combined "{{else if ...}}" / "{{else with ...}}" action.
		f.writeActionWithTrim("else "+chainKeyword+" "+f.pipeText(chain.Pipe), chain.StartTrimLeft, chain.StartTrimRight)
		f.body(chain.List, chain.StartTrimRight)
		if chain.ElseList != nil {
			f.formatElseList(chain, chain.ElseList)
		}
		return
	}
	f.writeActionWithTrim("else", parent.ElseTrimLeft, parent.ElseTrimRight)
	f.body(elseList, parent.ElseTrimRight)
}

func (f *formatter) defineNode(n *template.DefineNode) {
	hasBody := n.Body != nil && len(n.Body.Nodes) > 0

	// Empty body: emit on one line.
	if !hasBody {
		f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, n.StartTrimRight)
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
		return
	}

	outer := f.indent
	// Inline body unless every gap between items takes a break and block
	// layout is preferred. The edge gaps are trimmed below, so they need
	// not take one; a preserved first gap keeps its bytes instead.
	items, _ := splitChildren(n.Body.Nodes)
	forceTrim := f.cls.gaps[gapKey{n.Body, 0}].class != runPreserved
	block := f.blockLayoutPreferred(n.Body, items, itemsWidth(items))
	for i := 0; i <= len(items); i++ {
		edge := i == 0 || i == len(items)
		if !(edge && forceTrim) && !takesBreak(f.cls.gaps[gapKey{n.Body, i}]) {
			block = false
		}
	}
	if !block {
		f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, n.StartTrimRight)
		f.children(n.Body, n.Body.Nodes, listOptions{layout: inlineLayout, outer: outer})
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
		return
	}

	// Multi-line body: force trim markers on the boundary actions so the
	// formatter-introduced leading "\n\t" and trailing "\n" do not leak into
	// the rendered template output. Without these markers, callers of the
	// define would see surrounding whitespace that was not in their source.
	// Preserved runs (text that is not a page) keep their bytes instead.
	f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, n.StartTrimRight || forceTrim)
	f.indent++
	f.children(n.Body, n.Body.Nodes, listOptions{layout: blockLayout, outer: outer, trimEdges: forceTrim})
	f.indent = outer
	f.writeActionWithTrim("end", n.EndTrimLeft || forceTrim, n.EndTrimRight)
}

func (f *formatter) blockNode(n *template.BlockNode) {
	f.writeActionWithTrim("block "+strconv.Quote(n.Name)+" "+f.pipeText(n.Pipe), false, false)
	outer := f.indent
	if n.Body != nil && len(n.Body.Nodes) > 0 {
		f.indent++
		f.children(n.Body, n.Body.Nodes, listOptions{layout: blockLayout, outer: outer})
		f.indent = outer
	}
	f.writeActionWithTrim("end", false, false)
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

// estimateElementInlineLength estimates the rendered length of an element
// if it were formatted inline (open tag + children + close tag).
func estimateElementInlineLength(n *template.HTMLElementNode) int {
	// <tag attrs>children</tag>
	length := 1 + len(n.TagName) // "<tag"
	for _, attr := range n.Attr {
		length += contentWidth(attr.String())
	}
	length++ // ">"
	for _, child := range n.Children {
		length += contentWidth(child.String())
	}
	if !n.Void {
		length += 3 + len(n.TagName) // "</tag>"
	}
	return length
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

// Source formats a Go HTML template file. The file's final whitespace run is
// left exactly as written: Source never adds or removes a trailing newline.
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
