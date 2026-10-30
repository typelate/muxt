package format

import (
	"slices"
	"strings"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
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

// side is what may render on one side of a point: edge when every item
// is a certain line edge, unsure when some item is an element whose
// display is none or unknown, which forbids reflowing a run next to it
// whatever the other side is.
type side struct {
	edge, unsure bool
}

func (a side) and(b side) side {
	return side{edge: a.edge && b.edge, unsure: a.unsure || b.unsure}
}

var (
	edgeSide   = side{edge: true}
	opaqueSide = side{}
	unsureSide = side{unsure: true}
)

type memo struct {
	done     bool
	side     side
	nullable bool
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
	}
	left, right := opaqueSide, opaqueSide
	if leftTouch {
		left = c.leftSide(place{l, before})
	}
	if rightTouch {
		right = c.rightSide(place{l, after})
	}
	if !left.unsure && !right.unsure && (left.edge || right.edge) {
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

// The adjacency functions below answer "what may render next to this
// point?" Items that render nothing are looked through. Each is memoized;
// a cycle through control flow (a continue reaching its own range)
// answers opaque, which only makes runs less reflowable.

// enter returns the memo for key, with ok false when it is being computed
// already (a cycle).
func enter[K comparable](m map[K]*memo, key K) (result *memo, ok bool) {
	if r, seen := m[key]; seen {
		return r, r.done
	}
	r := &memo{}
	m[key] = r
	return r, true
}

// rightSide is what may render after position p.
func (c *classifier) rightSide(p place) side {
	r, ok := enter(c.right, p)
	if !ok {
		return opaqueSide
	}
	if r.done {
		return r.side
	}
	result, nullable := edgeSide, true
	for _, item := range p.l.items[p.i:] {
		s, n := c.firstOf(item)
		result = result.and(s)
		if !n {
			nullable = false
			break
		}
	}
	if nullable {
		result = result.and(c.endSide(p.l))
	}
	*r = memo{done: true, side: result}
	return result
}

// leftSide is what may render before position p.
func (c *classifier) leftSide(p place) side {
	r, ok := enter(c.left, p)
	if !ok {
		return opaqueSide
	}
	if r.done {
		return r.side
	}
	result, nullable := edgeSide, true
	for i := p.i - 1; i >= 0; i-- {
		s, n := c.lastOf(p.l.items[i])
		result = result.and(s)
		if !n {
			nullable = false
			break
		}
	}
	if nullable {
		result = result.and(c.startSide(p.l))
	}
	*r = memo{done: true, side: result}
	return result
}

// after is the position following item n in its list.
func (c *classifier) after(n template.Node) place {
	p := c.places[n]
	return place{p.l, p.i + 1}
}

// endSide is what may render after the end of l.
func (c *classifier) endSide(l *childList) side {
	switch n := l.owner.(type) {
	case *template.HTMLElementNode:
		switch c.display(n) {
		case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayAtomic:
			return edgeSide
		case analysis.DisplayInline, analysis.DisplayContents:
			return c.rightSide(c.after(n))
		case analysis.DisplayUnknown:
			return unsureSide
		}
	case *template.IfNode, *template.WithNode:
		return c.rightSide(c.after(n))
	case *template.RangeNode:
		if l.isElse {
			return c.rightSide(c.after(n))
		}
		// The body's end meets the next iteration's start or what
		// follows the range.
		next, _ := c.listFirst(l)
		return next.and(c.rightSide(c.after(n)))
	}
	// The tree root or a define or block body (the caller is unknown), or
	// the inside of an element whose display is none. The inside of an
	// element whose display is unknown is unsure: it may be inline next
	// to anything, or the browser may build it elsewhere.
	return opaqueSide
}

// startSide is what may render before the start of l.
func (c *classifier) startSide(l *childList) side {
	switch n := l.owner.(type) {
	case *template.HTMLElementNode:
		switch c.display(n) {
		case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayAtomic:
			return edgeSide
		case analysis.DisplayInline, analysis.DisplayContents:
			return c.leftSide(c.places[n])
		case analysis.DisplayUnknown:
			return unsureSide
		}
	case *template.IfNode, *template.WithNode:
		return c.leftSide(c.places[n])
	case *template.RangeNode:
		if l.isElse {
			return c.leftSide(c.places[n])
		}
		// The body's start meets what precedes the range, the previous
		// iteration's end, or a continue.
		result, _ := c.listLast(l)
		result = result.and(c.leftSide(c.places[n]))
		for _, exit := range c.exits[n] {
			if _, ok := exit.l.items[exit.i].(*template.ContinueNode); ok {
				result = result.and(c.leftSide(exit))
			}
		}
		return result
	}
	return opaqueSide
}

func (c *classifier) listFirst(l *childList) (side, bool) {
	result := edgeSide
	for _, item := range l.items {
		s, n := c.firstOf(item)
		result = result.and(s)
		if !n {
			return result, false
		}
	}
	return result, true
}

func (c *classifier) listLast(l *childList) (side, bool) {
	result := edgeSide
	for i := len(l.items) - 1; i >= 0; i-- {
		s, n := c.lastOf(l.items[i])
		result = result.and(s)
		if !n {
			return result, false
		}
	}
	return result, true
}

// firstOf is what may render first in n, and whether n may render
// nothing.
func (c *classifier) firstOf(n template.Node) (result side, nullable bool) {
	r, ok := enter(c.first, n)
	if !ok {
		return opaqueSide, false
	}
	if r.done {
		return r.side, r.nullable
	}
	switch n := n.(type) {
	case *template.HTMLElementNode:
		result, nullable = c.elementOutside(n, c.listFirst)
	case *template.IfNode, *template.WithNode, *template.RangeNode:
		result, nullable = c.branchEnds(branchOf(n), c.listFirst)
	case *template.BreakNode:
		result = opaqueSide
		if loop := c.target[n]; loop != nil {
			result = c.rightSide(c.after(loop))
		}
	case *template.ContinueNode:
		result = opaqueSide
		if loop := c.target[n]; loop != nil {
			result = c.endSide(c.lists[loop.List])
		}
	default:
		result, nullable = transparent(n)
	}
	*r = memo{done: true, side: result, nullable: nullable}
	return result, nullable
}

// lastOf is what may render last in n, and whether n may render nothing.
func (c *classifier) lastOf(n template.Node) (result side, nullable bool) {
	r, ok := enter(c.last, n)
	if !ok {
		return opaqueSide, false
	}
	if r.done {
		return r.side, r.nullable
	}
	switch n := n.(type) {
	case *template.HTMLElementNode:
		result, nullable = c.elementOutside(n, c.listLast)
	case *template.IfNode, *template.WithNode:
		result, nullable = c.branchEnds(branchOf(n), c.listLast)
	case *template.RangeNode:
		result, nullable = c.branchEnds(&n.BranchNode, c.listLast)
		// A break or continue ends the range next to what precedes it.
		for _, exit := range c.exits[n] {
			result = result.and(c.leftSide(exit))
		}
	case *template.BreakNode, *template.ContinueNode:
		// Nothing after it in its list renders; the range accounts for
		// what precedes it.
		result, nullable = edgeSide, false
	default:
		result, nullable = transparent(n)
	}
	*r = memo{done: true, side: result, nullable: nullable}
	return result, nullable
}

// elementOutside is how el looks from outside: a certain line edge, its
// children (display: contents), opaque (inline and atomic boxes), or
// unsure. An element whose display is none or unknown is unsure: a
// script may show a hidden element, and then it lays out by its own
// display, so no run next to it may be reflowed.
func (c *classifier) elementOutside(el *template.HTMLElementNode, ends func(*childList) (side, bool)) (side, bool) {
	switch c.display(el) {
	case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayNewline:
		return edgeSide, false
	case analysis.DisplayInline, analysis.DisplayAtomic:
		return opaqueSide, false
	case analysis.DisplayContents:
		if l, ok := c.lists[el]; ok {
			return ends(l)
		}
		return opaqueSide, false
	}
	return unsureSide, false
}

func (c *classifier) branchEnds(b *template.BranchNode, ends func(*childList) (side, bool)) (side, bool) {
	result, nullable := edgeSide, true
	if l, ok := c.lists[b.List]; ok {
		result, nullable = ends(l)
	}
	if b.ElseList == nil {
		return result, true
	}
	if l, ok := c.lists[b.ElseList]; ok {
		s, n := ends(l)
		return result.and(s), nullable || n
	}
	return result, nullable
}

// transparent classifies the remaining items: those that render nothing
// are looked through; text with content, actions, template calls and
// blocks are opaque.
func transparent(n template.Node) (side, bool) {
	switch n := n.(type) {
	case *template.CommentNode, *template.HTMLCommentNode, *template.DefineNode, *template.HTMLDoctypeNode:
		return edgeSide, true
	case *template.TextNode:
		if isRunOnly(n.Text) {
			return edgeSide, true
		}
	}
	return opaqueSide, false
}

// tableParents names, for each table-part tag, the parent elements in
// which the HTML parser keeps it. Elsewhere the parser drops the tag
// (HTML §13.2.6.4.7, "in body": a start tag tr, td, … is a parse error;
// ignore the token), so the element is no line edge.
var tableParents = map[string][]string{
	"caption":  {"table"},
	"colgroup": {"table"},
	"col":      {"table", "colgroup"},
	"tbody":    {"table"},
	"thead":    {"table"},
	"tfoot":    {"table"},
	"tr":       {"table", "tbody", "thead", "tfoot"},
	"td":       {"table", "tbody", "thead", "tfoot", "tr"},
	"th":       {"table", "tbody", "thead", "tfoot", "tr"},
}

// tableContexts are the elements whose content the parser reads "in
// table": other elements and text there are foster-parented, moved to
// just before the table (HTML §13.2.6.1). script, style and template are
// kept in place.
var tableContexts = map[string]bool{"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true}

var keptInTable = map[string]bool{"script": true, "style": true, "template": true}

// unsureParsedElsewhere sets the display of the elements whose box the
// HTML parser does not build where the template tree puts it to unknown,
// so the classifier treats them as unsure:
//   - a table part whose parent the parser does not keep it in (a table
//     part at the top of a define or block body has an unknown parent);
//   - an element foster-parented out of a table context, and the table
//     elements it is moved out of, since it renders next to the table;
//     text with content, or an action, template call or block that may
//     write text, marks the table elements the same way;
//   - an html, head or body element the parser does not build where it
//     stands: once the document holds content (text, an action, an
//     element that belongs in the body), and for a second one, the
//     parser drops the tag and its children are built where it stood
//     (HTML §13.2.6.4.7). A define or block body is taken to start a
//     document.
func unsureParsedElsewhere(root *template.ListNode, styles analysis.Whitespaces) {
	unsure := func(el *template.HTMLElementNode) {
		if ws, ok := styles[el]; ok {
			ws.Display = analysis.DisplayUnknown
			styles[el] = ws
		}
	}
	// doc is what the walk has seen of the document so far.
	var doc documentState
	// chain is the table-context elements from the nearest table down to
	// the current parent; it is empty outside a table context.
	var walk func(nodes []template.Node, parent string, chain []*template.HTMLElementNode)
	walk = func(nodes []template.Node, parent string, chain []*template.HTMLElementNode) {
		fosterParented := func() {
			for _, el := range chain {
				unsure(el)
			}
		}
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				walk(n.Children, parent, chain)
			case *template.TextNode:
				if !isRunOnly(n.Text) {
					doc.content = doc.content || !headTags[parent]
					if tableContexts[parent] {
						fosterParented()
					}
				}
			case *template.ActionNode, *template.TemplateNode:
				doc.content = doc.content || !headTags[parent]
				if tableContexts[parent] {
					fosterParented()
				}
			case *template.HTMLElementNode:
				tag := strings.ToLower(n.TagName)
				if !doc.inPlace(tag, parent) {
					unsure(n)
					walk(n.Children, parent, chain) // the parser drops the tag
					continue
				}
				allowed, isPart := tableParents[tag]
				switch {
				case isPart && !slices.Contains(allowed, parent):
					unsure(n)
					walk(n.Children, parent, chain) // the parser drops the tag
					continue
				case tableContexts[parent] && !isPart && tag != "table" && !keptInTable[tag]:
					unsure(n)
					fosterParented()
				}
				var inner []*template.HTMLElementNode
				if tableContexts[tag] {
					if tag != "table" {
						inner = chain
					}
					inner = append(slices.Clip(inner), n)
				}
				walk(n.Children, tag, inner)
			case *template.IfNode, *template.WithNode, *template.RangeNode:
				b := branchOf(n)
				if b.List != nil {
					walk(b.List.Nodes, parent, chain)
				}
				if b.ElseList != nil {
					walk(b.ElseList.Nodes, parent, chain)
				}
			case *template.DefineNode:
				if n.Body != nil {
					outer := doc
					doc = documentState{}
					walk(n.Body.Nodes, "", nil)
					doc = outer
				}
			case *template.BlockNode:
				doc.content = true
				if tableContexts[parent] {
					fosterParented()
				}
				if n.Body != nil {
					outer := doc
					doc = documentState{}
					walk(n.Body.Nodes, "", nil)
					doc = outer
				}
			}
		}
	}
	walk(root.Nodes, "", nil)
}

// documentState is what a walk in source order has seen of a document:
// whether body content started, and which of html, head and body the
// parser built.
type documentState struct {
	content, html, head, body bool
}

// headTags are the elements the parser puts in the head before the body
// starts (HTML §13.2.6.4.4): they do not start body content.
var headTags = map[string]bool{
	"base": true, "basefont": true, "bgsound": true, "link": true, "meta": true,
	"title": true, "noscript": true, "noframes": true, "style": true, "script": true, "template": true,
}

// inPlace reports whether the parser builds an element tag found under
// parent where it stands, and records it in d. Only html, head and body
// can be dropped: an html after any of them or after content, a head
// after a body or content, and a body after content or a second body.
func (d *documentState) inPlace(tag, parent string) bool {
	top := parent == "" || parent == "html"
	switch tag {
	case "html":
		ok := parent == "" && !d.content && !d.html && !d.head && !d.body
		d.html = true
		return ok
	case "head":
		ok := top && !d.content && !d.head && !d.body
		d.head = true
		return ok
	case "body":
		ok := top && !d.content && !d.body
		d.body = true
		return ok
	}
	if !headTags[tag] {
		d.content = true
	}
	return true
}

// repairedTags name the block elements that a start tag inside them can
// close (HTML §13.2.6.4.7 implied end tags): the browser's tree differs
// from the template tree there, so they are no stopping point when
// marking a misnested element's ancestors.
var repairedTags = map[string]bool{
	"p": true, "li": true, "dd": true, "dt": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// unsureMisnested sets the display of the elements the browser builds
// differently from the template tree to unknown, so the classifier
// treats them as unsure and leaves the runs next to and inside them as
// written (collapse-only at most). These are the elements carrying a
// structural diagnostic from analysis.CheckStructure (auto-closed,
// misnested, foster-parented, nested forms, links and buttons), the list
// items closesListItem and the tables closesTable finds, and their
// ancestors up to the element their start tag closes, then on up to the
// nearest clean block: a block
// element that carries no diagnostic and that no start tag closes.
// Whitespace there may land in a reopened formatting element or next to
// a box the template tree does not have. Every unclosed element is
// flagged too, an implied end tag (p, li) included: the formatter does
// not write the closing tag the source leaves out, the browser may end
// the element before the template tree does, and the template tree runs
// it to the end of its parent, so a run written after it would be its
// content on the next pass.
func unsureMisnested(root *template.ListNode, styles analysis.Whitespaces) {
	tree := &template.Tree{Root: root}
	flagged := make(map[lex.Pos]bool)
	for _, d := range analysis.CheckStructure(tree) {
		flagged[d.Pos] = true
	}
	// closes maps a list item to the ancestor its start tag closes; the
	// browser pops every element up to that ancestor.
	closes := make(map[*template.HTMLElementNode]*template.HTMLElementNode)
	// asWritten are the tables a nested table closes early: the parser
	// drops the table parts that follow, so the whole table is opaque.
	var asWritten []*template.HTMLElementNode
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		if !el.Void && !el.Closed {
			flagged[el.Start()] = true
		}
		if closed := closesListItem(el, path); closed != nil {
			flagged[el.Start()] = true
			closes[el] = closed
		}
		if closed := closesTable(el, path); closed != nil {
			flagged[el.Start()] = true
			closes[el] = closed
			asWritten = append(asWritten, closed)
		}
	})
	if len(flagged) == 0 {
		return
	}
	var marked []*template.HTMLElementNode
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok || !flagged[el.Start()] {
			return
		}
		marked = append(marked, el)
		closed := closes[el] // nil once reached, or when el closes nothing
		for i := len(path) - 1; i >= 0; i-- {
			switch p := path[i].(type) {
			case *template.HTMLElementNode:
				switch {
				case closed != nil:
					// Popped by el's start tag, clean or not.
					if p == closed {
						closed = nil
					}
				case cleanBlock(p, styles, flagged):
					return
				}
				marked = append(marked, p)
			case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
				return
			}
		}
	})
	for _, el := range marked {
		if ws, ok := styles[el]; ok {
			ws.Display = analysis.DisplayUnknown
			styles[el] = ws
		}
	}
	for _, el := range asWritten {
		if ws, ok := styles[el]; ok {
			ws.WhiteSpace = analysis.WhiteSpaceUnknown
			styles[el] = ws
		}
	}
}

// formattingMarkers are the elements that push a marker onto the list of
// active formatting elements (HTML §13.2.4.3): an unclosed formatting
// element inside one is not reconstructed past its end.
var formattingMarkers = map[string]bool{"td": true, "th": true, "caption": true, "template": true}

// unsureReopened sets the display of the elements that whitespace may
// reach after an unclosed formatting element (a, b, em, strong, …) to
// unknown: the element's ancestors and every element after it in
// document order, up to the end of the nearest formatting marker, define
// or block body, or the tree root. The browser keeps such an element in
// its list of active formatting elements past the block that closes it in
// the template tree, and any text there, a run the formatter writes
// included, reopens it (HTML §13.2.6.4.7, reconstruct the active
// formatting elements), so no gap there may take a break it did not have.
func unsureReopened(root *template.ListNode, styles analysis.Whitespaces) {
	tree := &template.Tree{Root: root}
	var (
		reaches []template.Node // per unclosed formatting element: where its reach ends; nil is the root
		marked  []*template.HTMLElementNode
	)
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		for _, r := range reaches {
			if r == nil || slices.Contains(path, r) {
				marked = append(marked, el)
				break
			}
		}
		if el.Closed || el.Void || !template.FormattingElements[strings.ToLower(el.TagName)] {
			return
		}
		reach := reachOf(path)
		reaches = append(reaches, reach)
		for i := len(path) - 1; i >= 0 && path[i] != reach; i-- {
			if p, ok := path[i].(*template.HTMLElementNode); ok {
				marked = append(marked, p)
			}
		}
	})
	for _, el := range marked {
		if ws, ok := styles[el]; ok {
			ws.Display = analysis.DisplayUnknown
			styles[el] = ws
		}
	}
}

// reachOf returns the nearest node in path that ends the reach of an
// unclosed formatting element: a formatting marker, or a define or block
// (whose callers are unknown). It returns nil when the reach is the tree
// root.
func reachOf(path []template.Node) template.Node {
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			if formattingMarkers[strings.ToLower(p.TagName)] {
				return p
			}
		case *template.DefineNode, *template.BlockNode:
			return p
		}
	}
	return nil
}

// closesTable returns the table that el, a table start tag, closes, or
// nil. Until a cell, caption or template opens, the content of a table
// is read "in table" even inside an element foster-parented out of it,
// and there a start tag table pops the open table and starts the new one
// after it (HTML §13.2.6.4.9), so what follows the nested table is no
// longer in a table.
func closesTable(el *template.HTMLElementNode, path []template.Node) *template.HTMLElementNode {
	if !strings.EqualFold(el.TagName, "table") {
		return nil
	}
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			switch strings.ToLower(p.TagName) {
			case "table":
				return p
			case "td", "th", "caption", "template":
				return nil
			}
		case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
			return nil
		}
	}
	return nil
}

func cleanBlock(el *template.HTMLElementNode, styles analysis.Whitespaces, flagged map[lex.Pos]bool) bool {
	ws, ok := styles[el]
	if !ok || flagged[el.Start()] || repairedTags[strings.ToLower(el.TagName)] {
		return false
	}
	return ws.Display == analysis.DisplayBlock || ws.Display == analysis.DisplayTableInternal
}

// closesListItem returns the open ancestor that the start tag of el, an
// li, dd or dt, closes, or nil. As in the browser (HTML §13.2.6.4.7), a
// start tag li closes the nearest li, and dd or dt the nearest dd or dt,
// unless a special element other than address, div or p comes first.
// analysis.CheckStructure finds only the case where that ancestor is the
// direct parent.
func closesListItem(el *template.HTMLElementNode, path []template.Node) *template.HTMLElementNode {
	tag := strings.ToLower(el.TagName)
	if tag != "li" && tag != "dd" && tag != "dt" {
		return nil
	}
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			ancestor := strings.ToLower(p.TagName)
			switch {
			case tag == "li" && ancestor == "li",
				tag != "li" && (ancestor == "dd" || ancestor == "dt"):
				return p
			case specialTags[ancestor] && ancestor != "address" && ancestor != "div" && ancestor != "p":
				return nil
			}
		case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
			return nil
		}
	}
	return nil
}

// specialTags are the elements of the special category (HTML §13.2.4.2),
// which stop the list-item scan of closesListItem, including the MathML
// and SVG ones (mi, mo, mn, ms, mtext, annotation-xml, foreignObject,
// desc, title). Those are matched by local name only: an HTML title has
// no li inside it to scan from, so the namespace never matters here.
var specialTags = map[string]bool{
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true,
	"annotation-xml": true, "foreignobject": true, "desc": true,
	"address": true, "applet": true, "area": true, "article": true, "aside": true,
	"base": true, "basefont": true, "bgsound": true, "blockquote": true, "body": true,
	"br": true, "button": true, "caption": true, "center": true, "col": true,
	"colgroup": true, "dd": true, "details": true, "dir": true, "div": true,
	"dl": true, "dt": true, "embed": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true, "frameset": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"head": true, "header": true, "hgroup": true, "hr": true, "html": true,
	"iframe": true, "img": true, "input": true, "keygen": true, "li": true,
	"link": true, "listing": true, "main": true, "marquee": true, "menu": true,
	"meta": true, "nav": true, "noembed": true, "noframes": true, "noscript": true,
	"object": true, "ol": true, "p": true, "param": true, "plaintext": true,
	"pre": true, "script": true, "search": true, "section": true, "select": true,
	"source": true, "style": true, "summary": true, "table": true, "tbody": true,
	"td": true, "template": true, "textarea": true, "tfoot": true, "th": true,
	"thead": true, "title": true, "tr": true, "track": true, "ul": true,
	"wbr": true, "xmp": true,
}
