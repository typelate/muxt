package format

import (
	"strings"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
)

// gap is the whitespace between two consecutive items of a child list
// (element children, branch list or else list, define/block body, tree
// root) or between a list boundary and its first or last item. text is
// nil for an empty gap. Whitespace-only text nodes are gaps, not items.
type gap struct {
	text       *template.TextNode
	start, end int // run bytes within text.Text
	class      runClass
}

type gapKey struct {
	list  template.Node // element, branch node list, define/block body, or tree root list
	index int           // gap before item index; len(items) is after the last
}

type classes struct {
	gaps   map[gapKey]gap
	runs   map[*template.TextNode][]gap // runs inside text with content, in order
	opaque []template.Node              // subtrees emitted from source, outermost only
}

// opaqueTagNames name the elements whose content is emitted from source
// whatever the CSS says: raw text and RCDATA elements, and the
// preformatted elements whose content the parser treats specially.
var opaqueTagNames = map[string]bool{
	"pre": true, "listing": true, "xmp": true, "plaintext": true, "textarea": true,
	"script": true, "style": true, "title": true, "iframe": true,
	"noembed": true, "noframes": true, "noscript": true,
}

// classify sorts every whitespace run under root by what the browser does
// with it (spec §6.3–6.5). A run is reflowable when every item that may
// render next to it on one side, through control flow, is a certain line
// edge. When root contains no HTML element the text is not a page, so
// every run is preserved.
func classify(root *template.ListNode, styles analysis.Whitespaces) classes {
	c := &classifier{
		styles: styles,
		lists:  make(map[template.Node]*childList),
		places: make(map[template.Node]place),
		exits:  make(map[*template.RangeNode][]place),
		target: make(map[template.Node]*template.RangeNode),
		first:  make(map[template.Node]*memo),
		last:   make(map[template.Node]*memo),
		left:   make(map[place]*memo),
		right:  make(map[place]*memo),
		out: classes{
			gaps: make(map[gapKey]gap),
			runs: make(map[*template.TextNode][]gap),
		},
	}
	c.build(root, root.Nodes, nil, false, analysis.WhiteSpaceCollapse, nil)
	for _, l := range c.order {
		c.classifyList(l)
	}
	return c.out
}

// listItems returns the items of a gapKey list: its children with
// document fragments flattened and whitespace-only text left out.
func listItems(list template.Node) []template.Node {
	items, _ := splitChildren(childrenOf(list))
	return items
}

func childrenOf(list template.Node) []template.Node {
	switch n := list.(type) {
	case *template.HTMLElementNode:
		return n.Children
	case *template.ListNode:
		return n.Nodes
	}
	return nil
}

// splitChildren flattens document fragments in children and separates
// items from the whitespace-only text between them.
func splitChildren(children []template.Node) (items []template.Node, gaps []*template.TextNode) {
	gaps = []*template.TextNode{nil}
	var walk func([]template.Node)
	walk = func(nodes []template.Node) {
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				walk(n.Children)
				continue
			case *template.TextNode:
				if isRunOnly(n.Text) {
					if gaps[len(gaps)-1] == nil {
						gaps[len(gaps)-1] = n
					}
					continue
				}
			}
			items = append(items, n)
			gaps = append(gaps, nil)
		}
	}
	walk(children)
	return items, gaps
}

// childList is a list of items with the gaps between them.
type childList struct {
	key    template.Node // the gapKey list
	items  []template.Node
	gaps   []*template.TextNode // whitespace-only text per gap, len(items)+1
	mode   analysis.WhiteSpace
	owner  template.Node // element, branch, define or block; nil for the tree root
	isElse bool          // the else list of owner
}

// place is the position before item i of l.
type place struct {
	l *childList
	i int
}

type memo struct {
	done, edge, nullable bool
}

type classifier struct {
	styles   analysis.Whitespaces
	lists    map[template.Node]*childList
	order    []*childList
	places   map[template.Node]place               // item → its position
	exits    map[*template.RangeNode][]place       // break and continue positions
	target   map[template.Node]*template.RangeNode // break or continue → its range
	elements bool                                  // the tree has an HTML element
	first    map[template.Node]*memo
	last     map[template.Node]*memo
	left     map[place]*memo
	right    map[place]*memo
	out      classes
}

func (c *classifier) build(key template.Node, children []template.Node, owner template.Node, isElse bool, mode analysis.WhiteSpace, loop *template.RangeNode) {
	items, gaps := splitChildren(children)
	l := &childList{key: key, items: items, gaps: gaps, mode: mode, owner: owner, isElse: isElse}
	c.lists[key] = l
	c.order = append(c.order, l)
	for i, item := range items {
		c.places[item] = place{l, i}
		switch n := item.(type) {
		case *template.HTMLElementNode:
			c.elements = true
			if c.isOpaque(n) {
				c.out.opaque = append(c.out.opaque, n)
				continue
			}
			c.build(n, n.Children, n, false, c.styles[n].WhiteSpace, loop)
		case *template.IfNode:
			c.buildBranch(&n.BranchNode, n, mode, loop, loop)
		case *template.WithNode:
			c.buildBranch(&n.BranchNode, n, mode, loop, loop)
		case *template.RangeNode:
			c.buildBranch(&n.BranchNode, n, mode, n, loop)
		case *template.DefineNode:
			c.build(n.Body, n.Body.Nodes, n, false, analysis.WhiteSpaceCollapse, nil)
		case *template.BlockNode:
			c.build(n.Body, n.Body.Nodes, n, false, analysis.WhiteSpaceCollapse, nil)
		case *template.BreakNode, *template.ContinueNode:
			if loop != nil {
				c.target[n] = loop
				c.exits[loop] = append(c.exits[loop], place{l, i})
			}
		}
	}
}

func (c *classifier) buildBranch(b *template.BranchNode, owner template.Node, mode analysis.WhiteSpace, bodyLoop, elseLoop *template.RangeNode) {
	if b.List != nil {
		c.build(b.List, b.List.Nodes, owner, false, mode, bodyLoop)
	}
	if b.ElseList != nil {
		c.build(b.ElseList, b.ElseList.Nodes, owner, true, mode, elseLoop)
	}
}

func (c *classifier) isOpaque(el *template.HTMLElementNode) bool {
	if opaqueTagNames[strings.ToLower(el.TagName)] {
		return true
	}
	ws, ok := c.styles[el]
	return !ok || ws.WhiteSpace == analysis.WhiteSpacePreserve || ws.WhiteSpace == analysis.WhiteSpaceUnknown
}

func (c *classifier) display(el *template.HTMLElementNode) analysis.Display {
	ws, ok := c.styles[el]
	if !ok {
		return analysis.DisplayUnknown
	}
	return ws.Display
}

func (c *classifier) classifyList(l *childList) {
	type run struct {
		text  *template.TextNode
		start int
	}
	taken := make(map[run]bool) // content text runs used as gaps
	for i := range l.gaps {
		g, leftTouch, rightTouch := c.gapRun(l, i)
		if g.text != nil {
			taken[run{g.text, g.start}] = true
		}
		g.class = c.runClassOf(l, i, i, g.text != nil, leftTouch, rightTouch)
		c.out.gaps[gapKey{l.key, i}] = g
	}
	for k, item := range l.items {
		text, ok := item.(*template.TextNode)
		if !ok {
			continue
		}
		var runs []gap
		for _, s := range splitRuns(text.Text) {
			if !s.space || taken[run{text, s.start}] {
				continue
			}
			leftTouch, rightTouch := s.start == 0, s.end == len(text.Text)
			runs = append(runs, gap{
				text: text, start: s.start, end: s.end,
				class: c.runClassOf(l, k, k+1, true, leftTouch, rightTouch),
			})
		}
		if len(runs) > 0 {
			c.out.runs[text] = runs
		}
	}
}

// gapRun finds the run in gap i of l: whitespace-only text, else the
// trailing run of the text item before it, else the leading run of the
// text item after it. leftTouch and rightTouch report whether the run
// reaches the gap's left and right sides rather than text content.
func (c *classifier) gapRun(l *childList, i int) (g gap, leftTouch, rightTouch bool) {
	if text := l.gaps[i]; text != nil {
		return gap{text: text, end: len(text.Text)}, true, true
	}
	if i > 0 {
		if text, ok := l.items[i-1].(*template.TextNode); ok {
			if s := splitRuns(text.Text); s[len(s)-1].space {
				return gap{text: text, start: s[len(s)-1].start, end: s[len(s)-1].end}, false, true
			}
		}
	}
	if i < len(l.items) {
		if text, ok := l.items[i].(*template.TextNode); ok {
			if s := splitRuns(text.Text); s[0].space {
				return gap{text: text, start: s[0].start, end: s[0].end}, true, false
			}
		}
	}
	return gap{}, true, true
}

// runClassOf classifies a run of l whose left side, when leftTouch, is
// the position before item before, and whose right side, when
// rightTouch, is the position before item after.
func (c *classifier) runClassOf(l *childList, before, after int, nonEmpty, leftTouch, rightTouch bool) runClass {
	switch {
	case !c.elements:
		return runPreserved
	case nonEmpty && (leftTouch && c.trimmedAfter(l, before) || rightTouch && c.trimmedBefore(l, after)):
		return runTrimmed
	case l.mode == analysis.WhiteSpacePreserveBreaks:
		return runPreserveBreaks
	case l.mode != analysis.WhiteSpaceCollapse:
		return runPreserved
	case leftTouch && c.leftEdge(place{l, before}) || rightTouch && c.rightEdge(place{l, after}):
		return runReflowable
	}
	return runCollapseOnly
}

// trimmedAfter reports whether a -}} ends what precedes position i of l.
func (c *classifier) trimmedAfter(l *childList, i int) bool {
	if i > 0 {
		return trimsRight(l.items[i-1])
	}
	switch n := l.owner.(type) {
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		b := branchOf(n)
		if l.isElse {
			return b.ElseTrimRight
		}
		return b.StartTrimRight
	case *template.DefineNode:
		return n.StartTrimRight
	case *template.BlockNode:
		return n.StartTrimRight
	}
	return false
}

// trimmedBefore reports whether a {{- starts what follows position i of l.
func (c *classifier) trimmedBefore(l *childList, i int) bool {
	if i < len(l.items) {
		return trimsLeft(l.items[i])
	}
	switch n := l.owner.(type) {
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		b := branchOf(n)
		if !l.isElse && b.ElseList != nil {
			return b.ElseTrimLeft
		}
		return b.EndTrimLeft
	case *template.DefineNode:
		return n.EndTrimLeft
	case *template.BlockNode:
		return n.EndTrimLeft
	}
	return false
}

func branchOf(n template.Node) *template.BranchNode {
	switch n := n.(type) {
	case *template.IfNode:
		return &n.BranchNode
	case *template.WithNode:
		return &n.BranchNode
	case *template.RangeNode:
		return &n.BranchNode
	}
	return nil
}

func trimsLeft(n template.Node) bool {
	switch n := n.(type) {
	case *template.ActionNode:
		return n.TrimLeft
	case *template.TemplateNode:
		return n.TrimLeft
	case *template.CommentNode:
		return n.TrimLeft
	case *template.BreakNode:
		return n.TrimLeft
	case *template.ContinueNode:
		return n.TrimLeft
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		return branchOf(n).StartTrimLeft
	case *template.DefineNode:
		return n.StartTrimLeft
	case *template.BlockNode:
		return n.StartTrimLeft
	}
	return false
}

func trimsRight(n template.Node) bool {
	switch n := n.(type) {
	case *template.ActionNode:
		return n.TrimRight
	case *template.TemplateNode:
		return n.TrimRight
	case *template.CommentNode:
		return n.TrimRight
	case *template.BreakNode:
		return n.TrimRight
	case *template.ContinueNode:
		return n.TrimRight
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		return branchOf(n).EndTrimRight
	case *template.DefineNode:
		return n.EndTrimRight
	case *template.BlockNode:
		return n.EndTrimRight
	}
	return false
}

// The adjacency functions below answer "is every item that may render
// next to this point a certain line edge?" Items that render nothing are
// looked through. Each is memoized; a cycle through control flow (a
// continue reaching its own range) answers false, which only makes runs
// less reflowable.

// enter returns the memo for key, or nil with ok false when it is being
// computed already (a cycle).
func enter[K comparable](m map[K]*memo, key K) (result *memo, ok bool) {
	if r, seen := m[key]; seen {
		return r, r.done
	}
	r := &memo{}
	m[key] = r
	return r, true
}

// rightEdge reports whether everything that may render after position p
// is a certain line edge.
func (c *classifier) rightEdge(p place) bool {
	r, ok := enter(c.right, p)
	if !ok {
		return false
	}
	if r.done {
		return r.edge
	}
	edge := true
	nullable := true
	for _, item := range p.l.items[p.i:] {
		e, n := c.firstOf(item)
		edge = edge && e
		if !n {
			nullable = false
			break
		}
	}
	if nullable {
		edge = edge && c.endEdge(p.l)
	}
	*r = memo{done: true, edge: edge}
	return edge
}

// leftEdge reports whether everything that may render before position p
// is a certain line edge.
func (c *classifier) leftEdge(p place) bool {
	r, ok := enter(c.left, p)
	if !ok {
		return false
	}
	if r.done {
		return r.edge
	}
	edge := true
	nullable := true
	for i := p.i - 1; i >= 0; i-- {
		e, n := c.lastOf(p.l.items[i])
		edge = edge && e
		if !n {
			nullable = false
			break
		}
	}
	if nullable {
		edge = edge && c.startEdge(p.l)
	}
	*r = memo{done: true, edge: edge}
	return edge
}

// after is the position following item n in its list.
func (c *classifier) after(n template.Node) place {
	p := c.places[n]
	return place{p.l, p.i + 1}
}

// endEdge reports whether what follows the end of l is a certain line edge.
func (c *classifier) endEdge(l *childList) bool {
	switch n := l.owner.(type) {
	case *template.HTMLElementNode:
		switch c.display(n) {
		case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayAtomic:
			return true
		case analysis.DisplayInline, analysis.DisplayContents:
			return c.rightEdge(c.after(n))
		}
	case *template.IfNode, *template.WithNode:
		return c.rightEdge(c.after(n))
	case *template.RangeNode:
		if l.isElse {
			return c.rightEdge(c.after(n))
		}
		// The body's end meets the next iteration's start or what
		// follows the range.
		next, _ := c.listFirst(l)
		return next && c.rightEdge(c.after(n))
	}
	return false // tree root, define or block body: the caller is unknown
}

// startEdge reports whether what precedes the start of l is a certain
// line edge.
func (c *classifier) startEdge(l *childList) bool {
	switch n := l.owner.(type) {
	case *template.HTMLElementNode:
		switch c.display(n) {
		case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayAtomic:
			return true
		case analysis.DisplayInline, analysis.DisplayContents:
			return c.leftEdge(c.places[n])
		}
	case *template.IfNode, *template.WithNode:
		return c.leftEdge(c.places[n])
	case *template.RangeNode:
		if l.isElse {
			return c.leftEdge(c.places[n])
		}
		// The body's start meets what precedes the range, the previous
		// iteration's end, or a continue.
		prev, _ := c.listLast(l)
		if !prev || !c.leftEdge(c.places[n]) {
			return false
		}
		for _, exit := range c.exits[n] {
			if _, ok := exit.l.items[exit.i].(*template.ContinueNode); ok && !c.leftEdge(exit) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *classifier) listFirst(l *childList) (edge, nullable bool) {
	edge = true
	for _, item := range l.items {
		e, n := c.firstOf(item)
		edge = edge && e
		if !n {
			return edge, false
		}
	}
	return edge, true
}

func (c *classifier) listLast(l *childList) (edge, nullable bool) {
	edge = true
	for i := len(l.items) - 1; i >= 0; i-- {
		e, n := c.lastOf(l.items[i])
		edge = edge && e
		if !n {
			return edge, false
		}
	}
	return edge, true
}

// firstOf reports whether every first rendered item of n is a certain
// line edge, and whether n may render nothing.
func (c *classifier) firstOf(n template.Node) (edge, nullable bool) {
	r, ok := enter(c.first, n)
	if !ok {
		return false, false
	}
	if r.done {
		return r.edge, r.nullable
	}
	switch n := n.(type) {
	case *template.HTMLElementNode:
		edge, nullable = c.elementOutside(n, c.listFirst)
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		edge, nullable = c.branchEnds(branchOf(n), c.listFirst)
	case *template.BreakNode:
		edge = c.target[n] != nil && c.rightEdge(c.after(c.target[n]))
	case *template.ContinueNode:
		edge = c.target[n] != nil && c.endEdge(c.lists[c.target[n].List])
	default:
		edge, nullable = transparent(n)
	}
	*r = memo{done: true, edge: edge, nullable: nullable}
	return edge, nullable
}

// lastOf reports whether every last rendered item of n is a certain line
// edge, and whether n may render nothing.
func (c *classifier) lastOf(n template.Node) (edge, nullable bool) {
	r, ok := enter(c.last, n)
	if !ok {
		return false, false
	}
	if r.done {
		return r.edge, r.nullable
	}
	switch n := n.(type) {
	case *template.HTMLElementNode:
		edge, nullable = c.elementOutside(n, c.listLast)
	case *template.IfNode, *template.WithNode:
		edge, nullable = c.branchEnds(branchOf(n), c.listLast)
	case *template.RangeNode:
		edge, nullable = c.branchEnds(&n.BranchNode, c.listLast)
		// A break or continue ends the range next to what precedes it.
		for _, exit := range c.exits[n] {
			edge = edge && c.leftEdge(exit)
		}
	case *template.BreakNode, *template.ContinueNode:
		// Nothing after it in its list renders; the range accounts for
		// what precedes it.
		edge, nullable = true, false
	default:
		edge, nullable = transparent(n)
	}
	*r = memo{done: true, edge: edge, nullable: nullable}
	return edge, nullable
}

// elementOutside is how el looks from outside: a certain line edge,
// transparent, its children (display: contents), or opaque.
func (c *classifier) elementOutside(el *template.HTMLElementNode, ends func(*childList) (bool, bool)) (edge, nullable bool) {
	switch c.display(el) {
	case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayNewline:
		return true, false
	case analysis.DisplayNone:
		return true, true
	case analysis.DisplayContents:
		if l, ok := c.lists[el]; ok {
			return ends(l)
		}
	}
	return false, false
}

func (c *classifier) branchEnds(b *template.BranchNode, ends func(*childList) (bool, bool)) (edge, nullable bool) {
	edge, nullable = true, true
	if l, ok := c.lists[b.List]; ok {
		edge, nullable = ends(l)
	}
	if b.ElseList == nil {
		return edge, true
	}
	if l, ok := c.lists[b.ElseList]; ok {
		e, n := ends(l)
		return edge && e, nullable || n
	}
	return edge, nullable
}

// transparent classifies the remaining items: those that render nothing
// are looked through; text with content, actions, template calls and
// blocks are opaque.
func transparent(n template.Node) (edge, nullable bool) {
	switch n := n.(type) {
	case *template.CommentNode, *template.HTMLCommentNode, *template.DefineNode, *template.HTMLDoctypeNode:
		return true, true
	case *template.TextNode:
		if isRunOnly(n.Text) {
			return true, true
		}
	}
	return false, false
}
