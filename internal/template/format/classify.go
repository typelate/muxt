package format

import (
	"maps"
	"slices"
	"strings"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
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
	// unsure are the opaque elements and the child lists whose
	// white-space the formatter cannot tell (unknown CSS, missing sheets,
	// dynamic style or class attributes, unknown call sites), as opposed
	// to those preserved by nature (pre, script, custom elements, SVG text)
	// or by known CSS.
	unsure map[template.Node]bool
}

// opaqueTagNames name the elements whose content is emitted from source
// whatever the CSS says: raw text and RCDATA elements, and the
// preformatted elements whose content the parser treats specially; the
// SVG text content elements, whose whitespace SVG handles by its own
// rules (xml:space, and SVG 1.1 deleting line breaks, turn indentation
// after a line break into a space); and the MathML token elements, whose
// rendering (mi auto-italic) depends on their exact characters and whose
// whitespace implementations treat differently.
var opaqueTagNames = map[string]bool{
	"pre": true, "listing": true, "xmp": true, "plaintext": true, "textarea": true,
	"script": true, "style": true, "title": true, "iframe": true,
	"noembed": true, "noframes": true, "noscript": true,
	"text": true, "tspan": true, "textpath": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true, "annotation-xml": true,
}

// classify sorts every whitespace run under root by what the browser does
// with it (v2 spec §2). Only white-space decides: a run whose owning
// element collapses white space may be reindented (runCollapse), a
// pre-line run keeps its line breaks (runPreserveBreaks), a run a trim
// marker removes is runTrimmed, and every other run is preserved.
// Display does not matter: the formatter keeps each run's kind (empty,
// spaces, line breaks), so whether a run touches a line edge never
// matters. When root contains no HTML element the text is not a page, so
// every run is preserved. Define and block bodies collapse at their top
// level; classifyModes gives them their call sites' modes.
func classify(root *template.ListNode, styles analysis.Whitespaces) classes {
	return classifyModes(root, styles, nil)
}

// classifyModes is classify with the top level of each define and block
// body in the mode modes holds for its name (callSiteModes), collapse
// for a name it lacks.
func classifyModes(root *template.ListNode, styles analysis.Whitespaces, modes map[string]rootMode) classes {
	c := &classifier{
		styles: styles,
		modes:  modes,
		out: classes{
			gaps:   make(map[gapKey]gap),
			runs:   make(map[*template.TextNode][]gap),
			unsure: make(map[template.Node]bool),
		},
	}
	c.build(root, root.Nodes, nil, false, analysis.WhiteSpaceCollapse, false)
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

type classifier struct {
	styles   analysis.Whitespaces
	modes    map[string]rootMode // define and block bodies' root modes
	order    []*childList
	elements bool // the tree has an HTML element
	out      classes
}

// build records the child list of key and, below it, of every element,
// branch, define and block. inSVG reports that the list is SVG content
// outside a foreignObject.
func (c *classifier) build(key template.Node, children []template.Node, owner template.Node, isElse bool, mode analysis.WhiteSpace, inSVG bool) {
	items, gaps := splitChildren(children)
	l := &childList{key: key, items: items, gaps: gaps, mode: mode, owner: owner, isElse: isElse}
	c.order = append(c.order, l)
	if mode == analysis.WhiteSpaceUnknown {
		c.out.unsure[key] = true
	}
	for _, item := range items {
		switch n := item.(type) {
		case *template.HTMLElementNode:
			c.elements = true
			tag := strings.ToLower(n.TagName)
			svg := inSVG && tag != "foreignobject" || tag == "svg"
			if c.isOpaque(n, svg) {
				c.out.opaque = append(c.out.opaque, n)
				if ws, ok := c.styles[n]; !opaqueByNature(n, svg) && (!ok || ws.WhiteSpace == analysis.WhiteSpaceUnknown) {
					c.out.unsure[n] = true
				}
				continue
			}
			c.build(n, n.Children, n, false, c.styles[n].WhiteSpace, svg)
		case *template.IfNode:
			c.buildBranch(&n.BranchNode, n, mode, inSVG)
		case *template.WithNode:
			c.buildBranch(&n.BranchNode, n, mode, inSVG)
		case *template.RangeNode:
			c.buildBranch(&n.BranchNode, n, mode, inSVG)
		case *template.DefineNode:
			c.build(n.Body, n.Body.Nodes, n, false, c.modes[n.Name].whiteSpace(), false)
		case *template.BlockNode:
			c.build(n.Body, n.Body.Nodes, n, false, c.modes[n.Name].whiteSpace(), false)
		}
	}
}

func (c *classifier) buildBranch(b *template.BranchNode, owner template.Node, mode analysis.WhiteSpace, inSVG bool) {
	if b.List != nil {
		c.build(b.List, b.List.Nodes, owner, false, mode, inSVG)
	}
	if b.ElseList != nil {
		c.build(b.ElseList, b.ElseList.Nodes, owner, true, mode, inSVG)
	}
}

// isOpaque reports whether el is written as it is: one of opaqueTagNames;
// a custom element (a hyphenated name) or another shadow host
// (isShadowHost), whose slotted text inherits shadow CSS the formatter
// cannot see; an SVG element with
// text of its own (svg reports that el is SVG content); or an element
// whose white-space is preserve or unknown.
func (c *classifier) isOpaque(el *template.HTMLElementNode, svg bool) bool {
	if opaqueByNature(el, svg) {
		return true
	}
	ws, ok := c.styles[el]
	return !ok || ws.WhiteSpace == analysis.WhiteSpacePreserve || ws.WhiteSpace == analysis.WhiteSpaceUnknown
}

// opaqueByNature reports whether el is written as it is whatever the
// CSS says (isOpaque): one of opaqueTagNames, a custom element or other
// shadow host, or SVG content with text of its own.
func opaqueByNature(el *template.HTMLElementNode, svg bool) bool {
	tag := strings.ToLower(el.TagName)
	return opaqueTagNames[tag] || strings.Contains(tag, "-") || isShadowHost(el) || svg && hasText(el)
}

// hasAttribute reports whether attrs, an element's attribute list, holds
// an attribute named name, in a template branch too. An attribute whose
// name is an action, or an action in attribute position, makes the
// element's white-space unknown already (analysis.ComputeWhitespace).
func hasAttribute(attrs []template.Node, name string) bool {
	for _, n := range attrs {
		switch n := n.(type) {
		case *template.HTMLAttributeNode:
			if n.Namespace != "" {
				continue
			}
			var key strings.Builder
			for _, k := range n.Key {
				if text, ok := k.(*template.TextNode); ok {
					key.Write(text.Text)
				}
			}
			if strings.EqualFold(strings.TrimSpace(key.String()), name) {
				return true
			}
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil && hasAttribute(b.List.Nodes, name) || b.ElseList != nil && hasAttribute(b.ElseList.Nodes, name) {
				return true
			}
		}
	}
	return false
}

// isShadowHost reports whether el may get a shadow root, under whose CSS
// the browser lays out its slotted light-DOM text: a customized built-in
// element (an is attribute, in a branch too), or the host of a
// declarative shadow root (a template child with shadowrootmode, or the
// legacy shadowroot). A template without either is an ordinary template:
// its contents are cloned into unknown places, and the formatter
// reindents them as collapsing content (WHATWG rulings).
func isShadowHost(el *template.HTMLElementNode) bool {
	if hasAttribute(el.Attr, "is") {
		return true
	}
	var visit func(nodes []template.Node) bool
	visit = func(nodes []template.Node) bool {
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				if visit(n.Children) {
					return true
				}
			case *template.HTMLElementNode:
				if strings.EqualFold(n.TagName, "template") && (hasAttribute(n.Attr, "shadowrootmode") || hasAttribute(n.Attr, "shadowroot")) {
					return true
				}
			case *template.IfNode, *template.WithNode, *template.RangeNode:
				b := branchOf(n)
				if b.List != nil && visit(b.List.Nodes) || b.ElseList != nil && visit(b.ElseList.Nodes) {
					return true
				}
			}
		}
		return false
	}
	return visit(el.Children)
}

// hasText reports whether el has a child, through branches, that may
// write text: text with content or an action.
func hasText(el *template.HTMLElementNode) bool {
	var visit func(nodes []template.Node) bool
	visit = func(nodes []template.Node) bool {
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				if visit(n.Children) {
					return true
				}
			case *template.TextNode:
				if !isRunOnly(n.Text) {
					return true
				}
			case *template.ActionNode, *template.TemplateNode, *template.BlockNode:
				return true
			case *template.IfNode, *template.WithNode, *template.RangeNode:
				b := branchOf(n)
				if b.List != nil && visit(b.List.Nodes) || b.ElseList != nil && visit(b.ElseList.Nodes) {
					return true
				}
			}
		}
		return false
	}
	return visit(el.Children)
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
// rightTouch, is the position before item after. Only the white-space of
// l's owner and the trim markers next to the run decide.
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
	return runCollapse
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

// tableParents names, for each table-part tag, the parent elements in
// which the HTML parser keeps it. Elsewhere the parser drops the tag
// (HTML §13.2.6.4.7, "in body": a start tag tr, td, … is a parse error;
// ignore the token) and builds its children in the parent.
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

// misplacement is an element the HTML parser builds, or whose content it
// builds, under owner instead of where the template tree puts it; owner
// nil is the tree root or a define or block body, which collapse, whose
// items are top. text reports that only el's text content moves (a table
// element whose text is foster-parented), not el.
type misplacement struct {
	el, owner *template.HTMLElementNode
	text      bool
	top       []template.Node
}

// whiteSpaceOf is the white-space of el, collapse for nil (the tree root
// or a define or block body), unknown for an element styles lacks.
func whiteSpaceOf(styles analysis.Whitespaces, el *template.HTMLElementNode) analysis.WhiteSpace {
	if el == nil {
		return analysis.WhiteSpaceCollapse
	}
	if ws, ok := styles[el]; ok {
		return ws.WhiteSpace
	}
	return analysis.WhiteSpaceUnknown
}

// unsure sets the white-space of el to unknown, so the classifier writes
// it as it is.
func unsure(styles analysis.Whitespaces, el *template.HTMLElementNode) {
	if ws, ok := styles[el]; ok {
		ws.WhiteSpace = analysis.WhiteSpaceUnknown
		styles[el] = ws
	}
}

// unsureParsedElsewhere applies parsedElsewhere: an element whose runs the
// browser may own by a white-space other than its own is unsure (spec §2:
// when the candidate owners differ, the most-preserving one wins; unknown
// is taken for it). Where both collapse, as on most pages, the runs stay
// reindentable: the formatter changes only amounts, never kinds, so the
// box the browser builds them next to does not matter.
//
// Selectors were matched in the template tree. When a white-space rule
// may match the moved element differently where the browser puts it
// (moved.MayMatch), the element is unsure, and so is the element that
// receives it, with its children, whose siblings and structure change:
// the owner, or each element at the top of the tree root or define body.
//
// When text in a head implies the body (parsedElsewhere) and the body does
// not certainly collapse white space (a body element whose white-space is
// not collapse, or none in root, whose implied body's is not known),
// unsureParsedElsewhere returns asIs: the runs from that text through the
// head's end and the head/body boundary are the body's, and the whole
// tree is written as it is.
func unsureParsedElsewhere(root *template.ListNode, styles analysis.Whitespaces, context []*template.Tree, moved analysis.MovedMatcher) (asIs bool) {
	misplaced, impliesBody := parsedElsewhere(root, styles, context)
	if impliesBody && !bodiesCollapse(root, styles) {
		return true
	}
	for _, m := range misplaced {
		var els []*template.HTMLElementNode
		if !m.text {
			els = []*template.HTMLElementNode{m.el}
		}
		if moved.MayMatch(els) {
			unsure(styles, m.el)
			if m.owner != nil {
				unsure(styles, m.owner)
				continue
			}
			items, _ := splitChildren(m.top)
			for _, item := range items {
				if el, ok := item.(*template.HTMLElementNode); ok {
					unsure(styles, el)
				}
			}
			continue
		}
		if whiteSpaceOf(styles, m.el) != whiteSpaceOf(styles, m.owner) {
			unsure(styles, m.el)
		}
	}
	return false
}

// bodiesCollapse reports whether root holds a body element and every one
// of them certainly collapses white space.
func bodiesCollapse(root *template.ListNode, styles analysis.Whitespaces) bool {
	found, ok := false, true
	template.Walk(&template.Tree{Root: root}, func(_ *template.Tree, n template.Node) {
		if el, isEl := n.(*template.HTMLElementNode); isEl && strings.EqualFold(el.TagName, "body") {
			found = true
			ok = ok && whiteSpaceOf(styles, el) == analysis.WhiteSpaceCollapse
		}
	})
	return found && ok
}

// parsedElsewhere finds the elements whose box, or content, the HTML
// parser does not build where the template tree puts it, each with the
// element it builds there under:
//   - a table part whose parent the parser does not keep it in (a table
//     part at the top of a define or block body has an unknown parent),
//     whose children are built in its parent;
//   - an element foster-parented out of a table context, and the table
//     elements it is moved out of, since it renders next to the table;
//     text with content, or an action that may write text, marks the
//     table elements the same way; each is built under the table's
//     parent;
//   - an html, head or body element the parser does not build where it
//     stands: once the document holds content (text, an action, an
//     element that belongs in the body), and for a second one, the
//     parser drops the tag and its children are built where it stood
//     (HTML §13.2.6.4.7), in its parent. A define or block body is
//     taken to start a document.
//
// A {{template}} call or a {{block}} stands for every non-empty body of
// that name in root and the context trees (the package's other
// templates), a block's own body included, each walked where it is
// called; the marks of all of them are kept. A recursive call is taken
// to write text, like an action. A call to a template defined nowhere
// is taken to write what belongs where it stands (head content in a
// head, table content in a table context) only when the file's html
// and body, or the table's parent, certainly collapse white space;
// otherwise it is taken to write text.
//
// impliesBody reports text, or an action or call that may write text, in
// a head or in html before a body: the browser closes the head there and
// implies the body, which owns the runs that follow up to the body tag.
func parsedElsewhere(root *template.ListNode, styles analysis.Whitespaces, context []*template.Tree) (out []misplacement, impliesBody bool) {
	collapses := func(el *template.HTMLElementNode) bool {
		ws, ok := styles[el]
		return ok && ws.WhiteSpace == analysis.WhiteSpaceCollapse
	}
	// defines are the non-empty bodies each defined name may resolve to,
	// from root and the context trees (the package's other templates):
	// which one html/template runs depends on parse order, so a call
	// stands for every one of them.
	defines := make(map[string][]*template.ListNode)
	collectDefines(root.Nodes, defines)
	for _, tree := range context {
		if tree != nil && tree.Root != nil {
			collectDefines(tree.Root.Nodes, defines)
		}
	}
	// A call defined nowhere is taken to write what belongs where it
	// stands only where guessing wrong cannot change rendering: the
	// document's html and body (for a call in the head), or the table's
	// parent (for a call in a table context), collapse white space.
	documentCollapses := documentElementsCollapse(root, collapses)
	tableParent := make(map[*template.HTMLElementNode]*template.HTMLElementNode)
	// calling holds the templates whose body the walk is in, so a
	// recursive call stops; walked memoizes in-place walks.
	calling := make(map[string]bool)
	type walkKey struct {
		name, parent string
		parentEl     *template.HTMLElementNode
		doc          documentState
		calling      string
	}
	walked := make(map[walkKey]documentState)
	// doc is what the walk has seen of the document so far.
	var doc documentState
	// top is the item list of the tree root or define or block body the
	// walk is in.
	top := root.Nodes
	// chain is the table-context elements from the nearest table down to
	// the current parent; it is empty outside a table context. parentEl
	// is the nearest enclosing element, nil at the top of root or a body.
	var walk func(nodes []template.Node, parent string, parentEl *template.HTMLElementNode, chain []*template.HTMLElementNode)
	walk = func(nodes []template.Node, parent string, parentEl *template.HTMLElementNode, chain []*template.HTMLElementNode) {
		// fosterTarget is the element foster-parented content is built
		// in: the parent of the nearest table.
		fosterTarget := func() *template.HTMLElementNode {
			if len(chain) == 0 {
				return nil
			}
			return tableParent[chain[0]]
		}
		fosterParented := func() {
			for _, el := range chain {
				out = append(out, misplacement{el: el, owner: fosterTarget(), text: true, top: top})
			}
		}
		// mayWriteText is the effect of text with content, or of an
		// action that may write text: it starts the body outside a head
		// tag (in a head, or in html before a body, the browser implies
		// the body there) and is foster-parented in a table context.
		mayWriteText := func() {
			if parent == "head" || parent == "html" && !doc.body {
				impliesBody = true
			}
			doc.content = doc.content || !headTags[parent]
			if tableContexts[parent] {
				fosterParented()
			}
		}
		// inPlace walks each body template name may run where it is
		// called, each from the same document state, and keeps every mark.
		inPlace := func(name string, bodies []*template.ListNode) {
			names := slices.Sorted(maps.Keys(calling))
			key := walkKey{name, parent, parentEl, doc, strings.Join(names, "\x00")}
			if out, ok := walked[key]; ok {
				doc = out
				return
			}
			start, out := doc, doc
			calling[name] = true
			for _, body := range bodies {
				doc = start
				walk(body.Nodes, parent, parentEl, chain)
				out = out.or(doc)
			}
			delete(calling, name)
			doc = out
			walked[key] = out
		}
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				walk(n.Children, parent, parentEl, chain)
			case *template.TextNode:
				if !isRunOnly(n.Text) {
					mayWriteText()
				}
			case *template.ActionNode:
				mayWriteText()
			case *template.TemplateNode:
				bodies, defined := defines[n.Name]
				switch {
				case calling[n.Name]:
					mayWriteText()
				case defined:
					inPlace(n.Name, bodies)
				case parent == "head" && documentCollapses:
				case tableContexts[parent] && collapses(tableParent[chain[0]]):
				default:
					mayWriteText()
				}
			case *template.HTMLElementNode:
				tag := strings.ToLower(n.TagName)
				if !doc.inPlace(tag, parent) {
					out = append(out, misplacement{el: n, owner: parentEl, top: top})
					walk(n.Children, parent, parentEl, chain) // the parser drops the tag
					continue
				}
				allowed, isPart := tableParents[tag]
				switch {
				case isPart && !slices.Contains(allowed, parent):
					out = append(out, misplacement{el: n, owner: parentEl, top: top})
					walk(n.Children, parent, parentEl, chain) // the parser drops the tag
					continue
				case tableContexts[parent] && !isPart && tag != "table" && !keptInTable[tag]:
					out = append(out, misplacement{el: n, owner: fosterTarget(), top: top})
					fosterParented()
				}
				var inner []*template.HTMLElementNode
				if tableContexts[tag] {
					if tag != "table" {
						inner = chain
					} else {
						tableParent[n] = parentEl
					}
					inner = append(slices.Clip(inner), n)
				}
				walk(n.Children, tag, n, inner)
			case *template.IfNode, *template.WithNode, *template.RangeNode:
				b := branchOf(n)
				if b.List != nil {
					walk(b.List.Nodes, parent, parentEl, chain)
				}
				if b.ElseList != nil {
					walk(b.ElseList.Nodes, parent, parentEl, chain)
				}
			case *template.DefineNode:
				if n.Body != nil {
					outer, outerTop := doc, top
					doc, top = documentState{}, n.Body.Nodes
					walk(n.Body.Nodes, "", nil, nil)
					doc, top = outer, outerTop
				}
			case *template.BlockNode:
				if n.Body == nil {
					continue
				}
				// The block writes its own body or a define of the same
				// name, unless the walk is in that body already.
				if calling[n.Name] {
					mayWriteText()
				} else {
					bodies := defines[n.Name]
					if !slices.Contains(bodies, n.Body) {
						bodies = append(slices.Clip(bodies), n.Body)
					}
					inPlace(n.Name, bodies)
				}
				// The body is also a define of its own.
				outer, outerTop := doc, top
				doc, top = documentState{}, n.Body.Nodes
				walk(n.Body.Nodes, "", nil, nil)
				doc, top = outer, outerTop
			}
		}
	}
	walk(root.Nodes, "", nil, nil)
	return out, impliesBody
}

// documentElementsCollapse reports whether root holds an html and a body
// element and every one of them collapses white space.
func documentElementsCollapse(root *template.ListNode, collapses func(*template.HTMLElementNode) bool) bool {
	found := map[string]bool{}
	ok := true
	var visit func(nodes []template.Node)
	visit = func(nodes []template.Node) {
		for _, n := range nodes {
			switch n := n.(type) {
			case *template.HTMLDocumentFragmentNode:
				visit(n.Children)
			case *template.HTMLElementNode:
				if tag := strings.ToLower(n.TagName); tag == "html" || tag == "body" {
					found[tag] = true
					ok = ok && collapses(n)
				}
				visit(n.Children)
			case *template.IfNode, *template.WithNode, *template.RangeNode:
				b := branchOf(n)
				if b.List != nil {
					visit(b.List.Nodes)
				}
				if b.ElseList != nil {
					visit(b.ElseList.Nodes)
				}
			case *template.DefineNode:
				if n.Body != nil {
					visit(n.Body.Nodes)
				}
			case *template.BlockNode:
				if n.Body != nil {
					visit(n.Body.Nodes)
				}
			}
		}
	}
	visit(root.Nodes)
	return ok && found["html"] && found["body"]
}

// or is what either walk saw of the document.
func (d documentState) or(e documentState) documentState {
	return documentState{
		content: d.content || e.content,
		html:    d.html || e.html,
		head:    d.head || e.head,
		body:    d.body || e.body,
	}
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

// unsureMisnested finds the elements the browser builds differently from
// the template tree, whose runs it may own by another element than the
// template tree does, and makes them unsure (white-space unknown, written
// as they are) when those candidate owners do not all have the same
// white-space (spec §2: the most-preserving one wins). Where they all
// collapse, as on most pages, the runs stay reindentable: the formatter
// changes only amounts, never kinds. These are the elements carrying a
// structural diagnostic from analysis.CheckStructure (auto-closed,
// misnested, foster-parented, nested forms, links and buttons), the list
// items closesListItem and the tables closesTable finds, and their
// ancestors up to the element their start tag closes, then on up to the
// nearest clean block: a block
// element that carries no diagnostic and that no start tag closes.
// Whitespace there may land in a reopened formatting element or in an
// element the template tree does not put it in. Every unclosed element is
// flagged too, an implied end tag (p, li) included: the formatter does
// not write the closing tag the source leaves out, the browser may end
// the element before the template tree does, and the template tree runs
// it to the end of its parent, so a run written after it would be its
// content on the next pass. What follows an element the browser holds
// open past its template end (heldOpen), or a start tag the parser read
// as text (an element only the browser has, which a later start tag such
// as <li> may close along with what the template tree nests in it), up
// to the nearest clean block, is flagged as well, as is what follows a p
// that the start tag of an unclosed element closes (closesParagraph): the
// element stays open past the p's end tag. When that start tag opens an
// unclosed preformatted element, what follows the p to the end of its
// parent is the element's content: that parent is written as it is, and
// at the tree root or in a define or block body unsureMisnested returns
// true, and the whole tree is written as it is. Selectors were matched in
// the template tree: when a white-space rule may match a group's elements
// (the flagged element included) differently in the browser's tree
// (moved.MayMatch), the whole group is unsure whatever its white-space.
func unsureMisnested(root *template.ListNode, styles analysis.Whitespaces, moved analysis.MovedMatcher) (asIs bool) {
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
	// unreadStarts hold a start tag the parser read as text: the
	// browser holds open an element the template tree does not have.
	unreadStarts := make(map[*template.TextNode]bool)
	// closesP are the start tags that close a p ancestor (closesParagraph).
	closesP := make(map[*template.HTMLElementNode]bool)
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		if text, ok := n.(*template.TextNode); ok {
			// A tag the parser read as text is an element the
			// template tree does not have: its parent is misnested,
			// and, for a start tag, written as it is.
			kind := unreadMarkupIn(text.Text)
			if kind == unreadNone || kind == unreadRetokenizes || inRawText(path) {
				return
			}
			if kind == unreadTag {
				unreadStarts[text] = true
				if hasUnreadStartTag(text.Text) {
					// What follows is its content to an ancestor
					// whose end tag pops it.
					if whole, parent := unreadPreformattedReach(path); whole {
						asIs = true
					} else {
						asWritten = append(asWritten, parent)
					}
				}
			}
			if parent := nearestElement(path); parent != nil {
				flagged[parent.Start()] = true
				if kind == unreadTag {
					asWritten = append(asWritten, parent)
				}
			}
			return
		}
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
		if closed, k := closesParagraph(el, path); closed != nil {
			flagged[el.Start()] = true
			closesP[el] = true
			if prev, ok := closes[el]; !ok || slices.Index(path, template.Node(prev)) > k {
				closes[el] = closed
			}
			if !el.Closed && preformats(el, styles) {
				// What follows the p to the end of its parent is
				// el's preformatted content.
				parent := elementAbove(path[:k])
				if parent == nil {
					asIs = true
				} else {
					asWritten = append(asWritten, parent)
				}
			}
		}
		if !el.Closed && !el.Void && specialTags[strings.ToLower(el.TagName)] && preformats(el, styles) {
			// An end tag that does not pop el leaves its
			// preformatted content running past it.
			if whole, parent := preformattedReach(path); whole {
				asIs = true
			} else if parent != nil {
				asWritten = append(asWritten, parent)
			}
		}
		if closed := closesTable(el, path); closed != nil {
			flagged[el.Start()] = true
			closes[el] = closed
			asWritten = append(asWritten, closed)
		}
	})
	if asIs {
		return true
	}
	if len(flagged) == 0 && len(unreadStarts) == 0 {
		return false
	}
	type reach struct {
		stop  template.Node // the node the reach ends at; nil is the tree root
		owner *ownerGroup
	}
	var (
		groups []*ownerGroup
		// held are, per element the browser keeps open past its end
		// in the template tree, its reach and the group of elements
		// whose runs it may own.
		held []reach
	)
	// mark starts a group with the ancestors of a flagged element (or of
	// a start tag read as text) up to the node the marking stops at.
	mark := func(path []template.Node, closed *template.HTMLElementNode) (*ownerGroup, template.Node) {
		g := new(ownerGroup)
		groups = append(groups, g)
		stop := markMisnestedAncestors(&g.members, path, closed, styles, flagged)
		g.add(stop)
		return g, stop
	}
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		if text, ok := n.(*template.TextNode); ok && unreadStarts[text] {
			if parent := nearestElement(path); parent != nil && parent.Closed && endTagPops[strings.ToLower(parent.TagName)] && !hasUnreadSpecialTag(text.Text) {
				return // the parent's end tag closes it; the parent is as written
			}
			g, stop := mark(path, nil)
			held = append(held, reach{stop, g})
			return
		}
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		for _, r := range held {
			if r.stop == nil || slices.Contains(path, r.stop) {
				r.owner.members = append(r.owner.members, el)
			}
		}
		if !flagged[el.Start()] {
			return
		}
		// el's own content stays el's; its ancestors' may move. el
		// itself may be matched differently where the browser puts it.
		g, stop := mark(path, closes[el])
		g.moved = append(g.moved, el)
		if heldOpen(el, path) || closesP[el] && !el.Closed {
			// What follows el in its reach may be el's content.
			g.members = append(g.members, el)
			held = append(held, reach{stop, g})
		}
	})
	for _, g := range groups {
		g.settle(styles, moved)
	}
	for _, el := range asWritten {
		unsure(styles, el)
	}
	return false
}

// endTagPops name the elements whose end tag pops every element opened
// inside them, an element only the browser has included (HTML §13.2.6.4.7
// "in body": the block end tags, </p>, </li>, </dd>, </dt> and the
// headings; and the table end tags). That holds only for an element that
// is not special: a special one may close the parent itself (<p><li/x>)
// or be a scope barrier that makes the parent's end tag ignored
// (<li><ul/x>, <p><button/x>, <table/x>), so hasUnreadSpecialTag rules
// the exception out.
var endTagPops = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "button": true,
	"center": true, "details": true, "dialog": true, "dir": true, "div": true,
	"dl": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"header": true, "hgroup": true, "listing": true, "main": true, "menu": true,
	"nav": true, "ol": true, "pre": true, "search": true, "section": true,
	"summary": true, "ul": true, "p": true, "li": true, "dd": true, "dt": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "caption": true, "tbody": true, "thead": true, "tfoot": true,
	"tr": true, "td": true, "th": true,
}

// hasUnreadSpecialTag reports whether text, which the template parser
// read as text, holds the start tag of a special element (specialTags).
func hasUnreadSpecialTag(text []byte) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' || !isASCIILetter(text[i+1]) {
			continue
		}
		end := i + 1
		for end < len(text) && !isTagNameEnd(text[end]) {
			end++
		}
		if specialTags[strings.ToLower(string(text[i+1:end]))] {
			return true
		}
	}
	return false
}

// ownerGroup is a set of elements whose runs the browser may build under
// any one of them, the tree root (or a define or block body) included
// when root is set.
type ownerGroup struct {
	members []*template.HTMLElementNode
	root    bool
	// moved are elements the browser builds elsewhere whose own runs
	// stay theirs: they count for the selectors, not the white-space.
	moved []*template.HTMLElementNode
}

// add puts n, where a marking stopped, in g: an element as a member, any
// other node (the tree root, a define, block, attribute or comment) as
// the root, which collapses.
func (g *ownerGroup) add(n template.Node) {
	if el, ok := n.(*template.HTMLElementNode); ok {
		g.members = append(g.members, el)
		return
	}
	g.root = true
}

// settle makes every member of g unsure when the members (and the root)
// do not all have the same white-space: a run may then be owned by an
// element whose white-space differs from the one the template tree
// gives it. When a white-space rule may match the members or moved
// elements differently in the browser's tree, all of them are unsure.
func (g *ownerGroup) settle(styles analysis.Whitespaces, moved analysis.MovedMatcher) {
	if all := slices.Concat(g.members, g.moved); moved.MayMatch(all) {
		for _, el := range all {
			unsure(styles, el)
		}
		return
	}
	modes := make(map[analysis.WhiteSpace]bool)
	if g.root {
		modes[analysis.WhiteSpaceCollapse] = true
	}
	for _, el := range g.members {
		modes[whiteSpaceOf(styles, el)] = true
	}
	if len(modes) < 2 {
		return
	}
	for _, el := range g.members {
		unsure(styles, el)
	}
}

// markMisnestedAncestors appends to marked the ancestors on path of a
// flagged element: those up to closed, the ancestor its start tag closes
// (nil when it closes none), then on up to the nearest clean block. It
// returns the node the marking stopped at: that clean block, a define,
// block, attribute or comment, or nil at the tree root.
func markMisnestedAncestors(marked *[]*template.HTMLElementNode, path []template.Node, closed *template.HTMLElementNode, styles analysis.Whitespaces, flagged map[lex.Pos]bool) template.Node {
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
				return p
			}
			*marked = append(*marked, p)
		case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
			return p
		}
	}
	return nil
}

// heldOpen reports whether the browser keeps el open past the end the
// template tree gives it: el is an unclosed special element below a
// formatting element with no other special element between them (others,
// such as span, may be), so the formatting element's end tag runs the
// adoption agency algorithm with el as the furthest block, the topmost
// special element below it, which stays on the stack of open elements
// (HTML §13.2.6.4.7). What follows, up to the nearest clean block, may be
// el's content or be split by a start tag that closes it
// (<b><li>0</b><b><li></li> </b>).
func heldOpen(el *template.HTMLElementNode, path []template.Node) bool {
	if el.Closed || el.Void || !specialTags[strings.ToLower(el.TagName)] {
		return false
	}
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			tag := strings.ToLower(p.TagName)
			if template.FormattingElements[tag] {
				return true
			}
			if specialTags[tag] {
				return false
			}
		case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
			return false
		}
	}
	return false
}

// nearestElement is the innermost element on path, or nil.
func nearestElement(path []template.Node) *template.HTMLElementNode {
	for i := len(path) - 1; i >= 0; i-- {
		if el, ok := path[i].(*template.HTMLElementNode); ok {
			return el
		}
	}
	return nil
}

// inRawText reports whether path is inside a raw text or RCDATA element,
// a comment or an attribute, where a < is not markup.
func inRawText(path []template.Node) bool {
	for _, n := range path {
		switch n := n.(type) {
		case *template.HTMLElementNode:
			if rawTextTags[strings.ToLower(n.TagName)] {
				return true
			}
		case *template.HTMLCommentNode, *template.HTMLAttributeNode:
			return true
		}
	}
	return false
}

// formattingMarkers are the elements that push a marker onto the list of
// active formatting elements (HTML §13.2.4.3): an unclosed formatting
// element inside one is not reconstructed past its end.
var formattingMarkers = map[string]bool{"td": true, "th": true, "caption": true, "template": true}

// unsureReopened makes unsure the elements whose runs may be built
// inside a reopened unclosed formatting element (a, b, em, strong, …)
// whose white-space differs from theirs: the element's ancestors and
// every element after it in document order, up to the end of the nearest
// formatting marker, define or block body, or the tree root. The browser
// keeps such an element in its list of active formatting elements past
// the block that closes it in the template tree, and any text there, a
// run included, reopens it (HTML §13.2.6.4.7, reconstruct the active
// formatting elements) as a child of the element the run is in. A
// formatting start tag the template parser read as text counts too; that
// element has no computed style, so its white-space is taken as unknown.
// Where the white-space is the same, as on most pages, the runs stay
// reindentable: the formatter changes only amounts, never kinds. When a
// white-space rule may match the formatting element or the elements in
// its reach differently in the browser's tree (moved.MayMatch: the clone
// is matched where the browser puts it, and the elements after it get a
// new ancestor), every element in the reach is unsure.
func unsureReopened(root *template.ListNode, styles analysis.Whitespaces, moved analysis.MovedMatcher) {
	tree := &template.Tree{Root: root}
	type reopened struct {
		stop     template.Node // where its reach ends; nil is the root
		mode     analysis.WhiteSpace
		elements []*template.HTMLElementNode // the formatting element (if any) and its reach
	}
	var (
		reaches []*reopened // per unclosed formatting element
		marked  []*template.HTMLElementNode
	)
	mark := func(r *reopened, el *template.HTMLElementNode) {
		r.elements = append(r.elements, el)
		if whiteSpaceOf(styles, el) != r.mode {
			marked = append(marked, el)
		}
	}
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		var mode analysis.WhiteSpace
		switch n := n.(type) {
		case *template.TextNode:
			// A formatting start tag the parser read as text (<a/x>)
			// is an unclosed formatting element to the browser.
			if inRawText(path) || !hasUnreadFormattingTag(n.Text) {
				return
			}
			mode = analysis.WhiteSpaceUnknown
		case *template.HTMLElementNode:
			for _, r := range reaches {
				if r.stop == nil || slices.Contains(path, r.stop) {
					mark(r, n)
				}
			}
			if n.Closed || n.Void || !template.FormattingElements[strings.ToLower(n.TagName)] {
				return
			}
			mode = whiteSpaceOf(styles, n)
		default:
			return
		}
		stop := reachOf(path)
		r := &reopened{stop: stop, mode: mode}
		reaches = append(reaches, r)
		for i := len(path) - 1; i >= 0 && path[i] != stop; i-- {
			if p, ok := path[i].(*template.HTMLElementNode); ok {
				mark(r, p)
			}
		}
		if el, ok := n.(*template.HTMLElementNode); ok {
			r.elements = append(r.elements, el) // matched where it is reopened
		}
	})
	for _, r := range reaches {
		if moved.MayMatch(r.elements) {
			marked = append(marked, r.elements...)
		}
	}
	for _, el := range marked {
		unsure(styles, el)
	}
}

// hasUnreadFormattingTag reports whether text, which the template parser
// read as text, holds the start tag of a formatting element (a, b, em,
// strong, …), as unreadMarkupIn finds start tags.
func hasUnreadFormattingTag(text []byte) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' || !isASCIILetter(text[i+1]) {
			continue
		}
		end := i + 1
		for end < len(text) && !isTagNameEnd(text[end]) {
			end++
		}
		if template.FormattingElements[strings.ToLower(string(text[i+1:end]))] {
			return true
		}
	}
	return false
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

// closesParagraph returns the p ancestor that the start tag of el
// closes and its index in path, or nil. A start tag that closes a p
// element (HTML §13.2.6.4.7: the block start tags, hr, the headings,
// li, dd, dt, table, ...) pops the nearest p in button scope, so what
// follows el in the p, and what follows the p's end tag (</p> then
// inserts an empty p), is not where the template tree puts it.
func closesParagraph(el *template.HTMLElementNode, path []template.Node) (*template.HTMLElementNode, int) {
	if !closesPTags[strings.ToLower(el.TagName)] {
		return nil, -1
	}
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			tag := strings.ToLower(p.TagName)
			if tag == "p" {
				return p, i
			}
			if buttonScopeBarriers[tag] {
				return nil, -1
			}
		case *template.DefineNode, *template.BlockNode, *template.HTMLAttributeNode, *template.HTMLCommentNode:
			return nil, -1
		}
	}
	return nil, -1
}

// closesPTags are the start tags that close a p element in button scope.
var closesPTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "center": true,
	"details": true, "dialog": true, "dir": true, "div": true, "dl": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hgroup": true, "hr": true, "main": true, "menu": true,
	"nav": true, "ol": true, "p": true, "pre": true, "listing": true,
	"search": true, "section": true, "summary": true, "table": true, "ul": true,
	"xmp": true, "plaintext": true, "li": true, "dd": true, "dt": true,
}

// buttonScopeBarriers end the button scope a p is looked up in (HTML
// §13.2.4.2), the MathML and SVG ones matched by local name.
var buttonScopeBarriers = map[string]bool{
	"applet": true, "caption": true, "html": true, "table": true, "td": true,
	"th": true, "marquee": true, "object": true, "template": true, "button": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true,
	"annotation-xml": true, "foreignobject": true, "desc": true, "title": true,
}

// preformats reports whether el's content keeps its whitespace: a
// preformatted or raw text element, or white-space other than collapse
// or unknown.
func preformats(el *template.HTMLElementNode, styles analysis.Whitespaces) bool {
	if opaqueTagNames[strings.ToLower(el.TagName)] {
		return true
	}
	ws, ok := styles[el]
	return !ok || ws.WhiteSpace != analysis.WhiteSpaceCollapse
}

// preformattedReach finds how far the browser runs an unclosed special
// preformatted element whose ancestors are path. The template tree ends
// it at the end tag of its nearest closed ancestor; when that end tag
// does not pop it (a formatting or ordinary end tag, which the adoption
// agency algorithm or the "any other end tag" rule leaves it open past,
// or </p> behind a scope barrier), what follows is its content up to an
// ancestor whose end tag does. preformattedReach returns that ancestor to
// be written as it is, or whole when there is none before the tree root
// or a define or block body. An {{if}}, {{with}} or {{range}} the element
// is in ends it as such an end tag does: the action is no end tag, so
// what follows the branch is its content. It returns nil and false when
// the nearest closed ancestor's end tag pops the element, or there is
// none.
func preformattedReach(path []template.Node) (whole bool, parent *template.HTMLElementNode) {
	closedSeen := false
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			if !p.Closed {
				continue
			}
			if endTagPopsAt(p, path[i+1:]) {
				if !closedSeen {
					return false, nil
				}
				return false, p
			}
			closedSeen = true
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			// The template tree ends it at the branch's end; the
			// browser, when the branch renders, does not.
			closedSeen = true
		case *template.DefineNode, *template.BlockNode:
			return closedSeen, nil
		}
	}
	return closedSeen, nil
}

// unreadPreformattedReach is preformattedReach for a start tag the
// parser read as text (white-space unknown, so treated as preserved), whose ancestors are path: the element
// opens inside the text's parent, so what follows the tag there is its
// content too. It returns the nearest closed ancestor whose end tag pops
// the element, to be written as it is, or whole when there is none
// before the tree root or a define or block body. A p does not count: the
// start tag may have closed it. Branches end nothing.
func unreadPreformattedReach(path []template.Node) (whole bool, parent *template.HTMLElementNode) {
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			if p.Closed && !strings.EqualFold(p.TagName, "p") && endTagPopsAt(p, path[i+1:]) {
				return false, p
			}
		case *template.DefineNode, *template.BlockNode:
			return true, nil
		}
	}
	return true, nil
}

// hasUnreadStartTag reports whether text, which the template parser read
// as text, holds a start tag. The element has no computed style, so its
// white-space is unknown: a pre, or any element author CSS may preserve
// whitespace in (span{white-space:pre}), keeps what follows as written.
func hasUnreadStartTag(text []byte) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] == '<' && isASCIILetter(text[i+1]) {
			return true
		}
	}
	return false
}

// endTagPopsAt reports whether the end tag of el pops every element
// below it, between (the elements from el's child down) included: el is
// one of endTagPops and no element between is a barrier of the scope its
// end tag looks it up in (HTML §13.2.4.2). The barriers are those of the
// default scope with the ones of button and list item scope added, which
// errs toward not popping.
func endTagPopsAt(el *template.HTMLElementNode, between []template.Node) bool {
	if !endTagPops[strings.ToLower(el.TagName)] {
		return false
	}
	for _, n := range between {
		if e, ok := n.(*template.HTMLElementNode); ok {
			tag := strings.ToLower(e.TagName)
			if buttonScopeBarriers[tag] || tag == "ol" || tag == "ul" {
				return false
			}
		}
	}
	return true
}

// elementAbove is the innermost element on path before any define or
// block, or nil.
func elementAbove(path []template.Node) *template.HTMLElementNode {
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *template.HTMLElementNode:
			return p
		case *template.DefineNode, *template.BlockNode:
			return nil
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

// collectDefines appends to defines the body of every define and block
// in nodes that is not empty (an empty one does not replace another of
// the same name in html/template), and records names whose bodies are
// all empty with no body.
func collectDefines(nodes []template.Node, defines map[string][]*template.ListNode) {
	record := func(name string, body *template.ListNode) {
		if body == nil {
			return
		}
		bodies := defines[name]
		if !template.IsEmptyTree(body) && !slices.Contains(bodies, body) {
			bodies = append(bodies, body)
		}
		defines[name] = bodies
		collectDefines(body.Nodes, defines)
	}
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.HTMLDocumentFragmentNode:
			collectDefines(n.Children, defines)
		case *template.HTMLElementNode:
			collectDefines(n.Children, defines)
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil {
				collectDefines(b.List.Nodes, defines)
			}
			if b.ElseList != nil {
				collectDefines(b.ElseList.Nodes, defines)
			}
		case *template.DefineNode:
			record(n.Name, n.Body)
		case *template.BlockNode:
			record(n.Name, n.Body)
		}
	}
}

// rootMode is the mode of a define or block body's top level, taken from
// its call sites (WHATWG rulings, define bodies (c)): the most-preserving
// white-space of the places its output lands, or verbatim when one of
// them is an attribute value, a start tag, or RCDATA or raw text, where
// every byte of the body is content. The zero value, collapse, is what a
// body with no call site in the parse set takes: a define executed from
// Go (a route), never called, or called from another package.
type rootMode struct {
	ws       analysis.WhiteSpace
	verbatim bool
}

// or is the more preserving of m and o.
func (m rootMode) or(o rootMode) rootMode {
	return rootMode{ws: max(m.ws, o.ws), verbatim: m.verbatim || o.verbatim}
}

// whiteSpace is the white-space the body's top level and elements
// inherit: a verbatim body preserves (and its defines are written as
// they are, verbatimDefines).
func (m rootMode) whiteSpace() analysis.WhiteSpace {
	if m.verbatim {
		return analysis.WhiteSpaceUnknown
	}
	return m.ws
}

// callSiteModes finds the root mode of each define and block name in
// trees over every {{template}} call and {{block}} of that name in
// trees, including those inside other bodies, whose own root mode then
// carries over (transitively). It raises the white-space of each body's
// elements in styles to its root mode, as inheritance would, which errs
// toward preserving for an element that sets a less preserving
// white-space of its own. A site's mode is the white-space of the
// element it stands in, or its body's root mode at a body's top level
// (collapse at a tree root); unknown inside a custom element, a shadow
// host, SVG text content or a MathML token element, whose white-space
// the formatter cannot see (isOpaque); verbatim in an attribute or in a
// raw text or RCDATA element. A {{block}} is a call site of its name
// where it stands, so it governs the defines that override it as well as
// its own body. Every body takes at least floor: unknown when call
// sites may be missing (the package's templates could not all be read).
func callSiteModes(trees []*template.Tree, styles analysis.Whitespaces, floor analysis.WhiteSpace) map[string]rootMode {
	modes := make(map[string]rootMode)
	for {
		w := siteWalk{styles: styles, modes: modes, out: make(map[string]rootMode)}
		if floor != analysis.WhiteSpaceCollapse {
			for _, t := range trees {
				template.Walk(t, func(_ *template.Tree, n template.Node) {
					switch n := n.(type) {
					case *template.DefineNode:
						w.out[n.Name] = rootMode{ws: floor}
					case *template.BlockNode:
						w.out[n.Name] = rootMode{ws: floor}
					}
				})
			}
		}
		for _, t := range trees {
			if t != nil && t.Root != nil {
				w.walk(t.Root.Nodes, siteContext{})
			}
		}
		for _, t := range trees {
			if t != nil && t.Root != nil {
				raiseBodies(t.Root.Nodes, analysis.WhiteSpaceCollapse, styles, w.out)
			}
		}
		// Modes and styles only grow toward preserving, so this ends.
		if maps.Equal(w.out, modes) {
			return modes
		}
		modes = w.out
	}
}

// siteContext is where a siteWalk is: the mode a call there has, short
// of the nearest element's white-space.
type siteContext struct {
	floor    analysis.WhiteSpace // the least preserving mode a call here has
	verbatim bool                // a call here is written into an attribute, raw text or RCDATA
	el       *template.HTMLElementNode
	svg      bool // SVG content outside a foreignObject
}

type siteWalk struct {
	styles analysis.Whitespaces
	modes  map[string]rootMode // the root modes found so far
	out    map[string]rootMode // the root modes this walk finds
}

func (w *siteWalk) site(name string, ctx siteContext) {
	m := rootMode{ws: ctx.floor, verbatim: ctx.verbatim}
	if ctx.el != nil {
		m.ws = max(m.ws, whiteSpaceOf(w.styles, ctx.el))
	}
	w.out[name] = w.out[name].or(m)
}

// body walks the body of the define or block name from its own top.
func (w *siteWalk) body(name string, body *template.ListNode) {
	if body == nil {
		return
	}
	m := w.modes[name]
	w.walk(body.Nodes, siteContext{floor: m.whiteSpace(), verbatim: m.verbatim})
}

func (w *siteWalk) walk(nodes []template.Node, ctx siteContext) {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.ListNode:
			if n != nil {
				w.walk(n.Nodes, ctx)
			}
		case *template.HTMLDocumentFragmentNode:
			w.walk(n.Children, ctx)
		case *template.HTMLAttributeNode:
			w.walk(n.Key, ctx)
			w.walk(n.Value, ctx)
		case *template.HTMLElementNode:
			attrs := ctx
			attrs.verbatim = true
			w.walk(n.Attr, attrs)
			tag := strings.ToLower(n.TagName)
			inner := ctx
			inner.el = n
			inner.svg = ctx.svg && tag != "foreignobject" || tag == "svg"
			switch {
			case rawTextTags[tag]:
				inner.verbatim = true
			case opaqueTagNames[tag] || strings.Contains(tag, "-") || isShadowHost(n) || inner.svg && hasText(n):
				inner.floor = analysis.WhiteSpaceUnknown
			}
			w.walk(n.Children, inner)
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil {
				w.walk(b.List.Nodes, ctx)
			}
			if b.ElseList != nil {
				w.walk(b.ElseList.Nodes, ctx)
			}
		case *template.TemplateNode:
			w.site(n.Name, ctx)
		case *template.DefineNode:
			w.body(n.Name, n.Body)
		case *template.BlockNode:
			w.site(n.Name, ctx)
			w.body(n.Name, n.Body)
		}
	}
}

// raiseBodies raises the white-space of the elements in nodes, which
// inherit ws, and in the define and block bodies there, which inherit
// their root modes, to at least that.
func raiseBodies(nodes []template.Node, ws analysis.WhiteSpace, styles analysis.Whitespaces, modes map[string]rootMode) {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.ListNode:
			if n != nil {
				raiseBodies(n.Nodes, ws, styles, modes)
			}
		case *template.HTMLDocumentFragmentNode:
			raiseBodies(n.Children, ws, styles, modes)
		case *template.HTMLAttributeNode:
			raiseBodies(n.Value, ws, styles, modes)
		case *template.HTMLElementNode:
			if s, ok := styles[n]; ok && s.WhiteSpace < ws {
				s.WhiteSpace = ws
				styles[n] = s
			}
			raiseBodies(n.Attr, ws, styles, modes)
			raiseBodies(n.Children, ws, styles, modes)
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil {
				raiseBodies(b.List.Nodes, ws, styles, modes)
			}
			if b.ElseList != nil {
				raiseBodies(b.ElseList.Nodes, ws, styles, modes)
			}
		case *template.DefineNode:
			if n.Body != nil {
				raiseBodies(n.Body.Nodes, modes[n.Name].whiteSpace(), styles, modes)
			}
		case *template.BlockNode:
			if n.Body != nil {
				raiseBodies(n.Body.Nodes, modes[n.Name].whiteSpace(), styles, modes)
			}
		}
	}
}

// verbatimDefines adds to bodies the defines in root whose root mode is
// verbatim: the emitter writes them as they are.
func verbatimDefines(root *template.ListNode, modes map[string]rootMode, bodies map[*template.DefineNode]bool) {
	template.Walk(&template.Tree{Root: root}, func(_ *template.Tree, n template.Node) {
		if d, ok := n.(*template.DefineNode); ok && modes[d.Name].verbatim {
			bodies[d] = true
		}
	})
}
