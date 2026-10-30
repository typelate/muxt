// Package format pretty-prints Go HTML template source for muxt fmt. It
// formats the syntax tree produced by muxt's own template parser, keeping
// HTML structure and template actions in a consistent layout.
package format

import (
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// inlineElements is the set of HTML elements that stay in text flow.
var inlineElements = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true,
	"br": true, "cite": true, "code": true, "data": true, "dfn": true,
	"em": true, "i": true, "kbd": true, "mark": true, "q": true,
	"rp": true, "rt": true, "ruby": true, "s": true, "samp": true,
	"small": true, "span": true, "strong": true, "sub": true, "sup": true,
	"time": true, "u": true, "var": true, "wbr": true,
}

// whitespaceSensitiveElements preserve their content exactly as written.
var whitespaceSensitiveElements = map[string]bool{
	"pre":       true,
	"listing":   true,
	"xmp":       true,
	"plaintext": true,
	"textarea":  true,
}

// rawTextElements have content that should not be reformatted.
var rawTextElements = map[string]bool{
	"script":   true,
	"style":    true,
	"textarea": true,
	"title":    true,
}

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
	f.node(root)
	result := cleanUnnecessaryTrim(f.sb.String(), f.leftDelim, f.rightDelim)
	return cleanWhitespace(result)
}

type formatter struct {
	sb              *strings.Builder
	indent          int
	preserveContent bool
	atLineStart     bool
	leftDelim       string
	rightDelim      string
	maxLineWidth    int
	maxInlineAttrs  int
}

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

// writeActionWithTrim writes a template action with optional trim markers.
func (f *formatter) writeActionWithTrim(content string, trimLeft, trimRight bool) {
	f.writeString(f.leftDelim)
	if trimLeft {
		f.writeString("- ")
	}
	f.writeString(content)
	if trimRight {
		f.writeString(" -")
	}
	f.writeString(f.rightDelim)
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
		f.textNode(n)
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

func (f *formatter) listNode(n *template.ListNode) {
	if n == nil {
		return
	}
	for _, child := range n.Nodes {
		f.node(child)
	}
}

func (f *formatter) documentFragment(n *template.HTMLDocumentFragmentNode) {
	f.formatChildren(n.Children)
}

func (f *formatter) textNode(n *template.TextNode) {
	if f.preserveContent {
		f.writeString(string(n.Text))
		return
	}
	// Whitespace-only text nodes between block siblings are dropped
	// (formatter controls structural whitespace).
	text := string(n.Text)
	if strings.TrimSpace(text) == "" {
		return
	}
	f.writeString(text)
}

func (f *formatter) htmlElement(n *template.HTMLElementNode) {
	if f.preserveContent {
		f.writeString(n.String())
		return
	}
	tag := strings.ToLower(n.TagName)
	isInline := inlineElements[tag]
	isPreserve := whitespaceSensitiveElements[tag] || rawTextElements[tag]
	isBlock := !isInline

	// Open tag (positioning handled by formatChildren)
	f.writeString("<")
	f.writeString(n.TagName)
	f.formatAttributes(n)
	if n.Void {
		f.writeString(">")
		return
	}
	f.writeString(">")

	if isPreserve {
		// Pass through children unchanged
		for _, child := range n.Children {
			f.writeString(child.String())
		}
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
		return
	}

	if len(n.Children) == 0 {
		// Empty element: closing tag on same line
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
		return
	}

	allInline := allChildrenInline(n.Children)
	hasLineBreaks := childrenHaveLineBreaks(n.Children)
	tooLong := estimateElementInlineLength(n) > f.maxLineWidth

	// Block layout when:
	// - Source has line breaks (multi-line content the author intentionally structured), OR
	// - Element is too long AND children include block-level elements.
	// When all children are inline and the only trigger is length, keep inline
	// layout — block layout would introduce newlines/indentation that change
	// rendered TextContent (e.g. <p>long prose...</p> should not become
	// <p>\n\tlong prose...\n</p>).
	useBlock := isBlock && (hasLineBreaks || (tooLong && !allInline))

	if useBlock {
		f.indent++
		if !hasLineBreaks {
			f.formatChildrenBoundaryTrim(n.Children)
		} else {
			f.formatChildren(n.Children)
		}
		f.indent--
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
	} else {
		// All inline: keep on same line
		f.formatChildrenInline(n.Children)
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
	}
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
	f.writeString(f.leftDelim)
	if n.TrimLeft {
		f.writeString("- ")
	}
	f.writePipe(n.Pipe)
	if n.TrimRight {
		f.writeString(" -")
	}
	f.writeString(f.rightDelim)
}

func (f *formatter) templateNode(n *template.TemplateNode) {
	f.writeString(f.leftDelim)
	if n.TrimLeft {
		f.writeString("- ")
	}
	f.writeString("template ")
	f.writeString(strconv.Quote(n.Name))
	if n.Pipe != nil {
		f.sb.WriteByte(' ')
		f.writePipe(n.Pipe)
	}
	if n.TrimRight {
		f.writeString(" -")
	}
	f.writeString(f.rightDelim)
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
	// Positioning handled by formatChildren.
	f.writeActionWithTrim(keyword+" "+f.pipeText(n.Pipe), n.StartTrimLeft, n.StartTrimRight)

	bodyInline := false
	if n.List != nil && len(n.List.Nodes) > 0 {
		if f.canInlineBody(n.List.Nodes) {
			f.formatChildrenInline(n.List.Nodes)
			bodyInline = true
		} else {
			if !n.StartTrimRight {
				f.indent++
			}
			f.formatChildren(n.List.Nodes)
			if !n.StartTrimRight {
				f.indent--
			}
		}
	}

	hasElse := n.ElseList != nil && len(n.ElseList.Nodes) > 0
	if hasElse {
		bodyInline = f.formatElseList(n, n.ElseList, bodyInline)
	}

	if bodyInline {
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
	} else {
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
	}
}

// formatElseList handles the else portion of a branch, recursing for else-if chains.
// prevBodyInline indicates whether the preceding body was formatted inline.
// It returns true when the last body was formatted inline.
func (f *formatter) formatElseList(parent *template.BranchNode, elseList *template.ListNode, prevBodyInline bool) bool {
	if chain, chainKeyword := elseChain(elseList); chain != nil {
		if !prevBodyInline {
			if !f.atLineStart {
				f.newline()
			}
			f.writeIndent()
		}
		// For else-if/else-with chains, the inner BranchNode carries the trim
		// for the combined "{{else if ...}}" / "{{else with ...}}" action.
		f.writeActionWithTrim("else "+chainKeyword+" "+f.pipeText(chain.Pipe), chain.StartTrimLeft, chain.StartTrimRight)
		bodyInline := false
		if chain.List != nil && len(chain.List.Nodes) > 0 {
			if f.canInlineBody(chain.List.Nodes) {
				f.formatChildrenInline(chain.List.Nodes)
				bodyInline = true
			} else {
				if !chain.StartTrimRight {
					f.indent++
				}
				f.formatChildren(chain.List.Nodes)
				if !chain.StartTrimRight {
					f.indent--
				}
			}
		}
		if chain.ElseList != nil && len(chain.ElseList.Nodes) > 0 {
			return f.formatElseList(chain, chain.ElseList, bodyInline)
		}
		return bodyInline
	}

	if !prevBodyInline {
		if !f.atLineStart {
			f.newline()
		}
		f.writeIndent()
	}
	f.writeActionWithTrim("else", parent.ElseTrimLeft, parent.ElseTrimRight)

	if f.canInlineBody(elseList.Nodes) {
		f.formatChildrenInline(elseList.Nodes)
		return true
	}

	if !parent.ElseTrimRight {
		f.indent++
	}
	f.formatChildren(elseList.Nodes)
	if !parent.ElseTrimRight {
		f.indent--
	}
	return false
}

func (f *formatter) defineNode(n *template.DefineNode) {
	// Positioning handled by formatChildren.
	hasBody := n.Body != nil && len(n.Body.Nodes) > 0

	// Empty body: emit on one line.
	if !hasBody {
		f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, n.StartTrimRight)
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
		return
	}

	// Inline body: emit on one line. Preserves rendered output exactly.
	if allChildrenInline(n.Body.Nodes) && f.canInlineBody(n.Body.Nodes) {
		f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, n.StartTrimRight)
		f.formatChildrenInline(n.Body.Nodes)
		f.writeActionWithTrim("end", n.EndTrimLeft, n.EndTrimRight)
		return
	}

	// Multi-line body: force trim markers on the boundary actions so the
	// formatter-introduced leading "\n\t" and trailing "\n" do not leak into
	// the rendered template output. Without these markers, callers of the
	// define would see surrounding whitespace that was not in their source.
	f.writeActionWithTrim("define "+strconv.Quote(n.Name), n.StartTrimLeft, true)
	f.indent++
	f.formatChildren(n.Body.Nodes)
	f.indent--

	if !f.atLineStart {
		f.newline()
	}
	f.writeIndent()
	f.writeActionWithTrim("end", true, n.EndTrimRight)
}

func (f *formatter) blockNode(n *template.BlockNode) {
	f.writeActionWithTrim("block "+strconv.Quote(n.Name)+" "+f.pipeText(n.Pipe), false, false)

	if n.Body != nil && len(n.Body.Nodes) > 0 {
		f.indent++
		f.formatChildren(n.Body.Nodes)
		f.indent--
	}

	if !f.atLineStart {
		f.newline()
	}
	f.writeIndent()
	f.writeActionWithTrim("end", false, false)
}

func (f *formatter) commentNode(n *template.CommentNode) {
	f.writeString(f.leftDelim)
	if n.TrimLeft {
		f.writeString("- ")
	}
	f.writeString(n.Text)
	if n.TrimRight {
		f.writeString(" -")
	}
	f.writeString(f.rightDelim)
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

// formatChildren formats a list of child nodes with block-level structure.
// It handles newline and indentation for every child so that individual
// node methods only emit their own content.
func (f *formatter) formatChildren(children []template.Node) {
	f.formatChildrenImpl(children, nil, nil)
}

// formatChildrenBoundaryTrim is like formatChildren but adds trim markers to
// action/template nodes at the boundaries. This preserves the rendered output
// when the formatter breaks originally-inline content into block layout.
func (f *formatter) formatChildrenBoundaryTrim(children []template.Node) {
	first, last := significantBoundaries(children)
	f.formatChildrenImpl(children, first, last)
}

func (f *formatter) formatChildrenImpl(children []template.Node, trimFirst, trimLast template.Node) {
	var lastSignificant template.Node
	var continueInline bool
	for _, child := range children {
		// Extra blank line between adjacent defines.
		_, isDefine := child.(*template.DefineNode)
		_, lastWasDefine := lastSignificant.(*template.DefineNode)
		if isDefine && lastWasDefine {
			if !f.atLineStart {
				f.newline()
			}
			f.newline()
		}

		// Text nodes: trim whitespace and write on own line.
		if tn, ok := child.(*template.TextNode); ok {
			text := strings.TrimSpace(string(tn.Text))
			if text == "" {
				// Preserve single space between inline siblings
				if lastSignificant != nil && isInlineNode(lastSignificant) {
					if nextSibling := nextNonWhitespace(children, child); nextSibling != nil && isInlineNode(nextSibling) {
						if !isVoidHTMLElement(lastSignificant) || !isVoidHTMLElement(nextSibling) {
							continueInline = true
						}
						continue
					}
				}
				// Whitespace-only text with blank lines: emit one blank line
				if hasBlankLine(string(tn.Text)) && lastSignificant != nil {
					if !f.atLineStart {
						f.newline()
					}
					f.newline()
				}
				continue
			}
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					// Blank line between content lines: emit one blank line
					if !f.atLineStart {
						f.newline()
					}
					f.newline()
					continue
				}
				if continueInline || (i == 0 && lastSignificant != nil && isInlineNode(lastSignificant)) {
					f.writeString(" ")
					f.writeString(trimmed)
					continueInline = false
				} else {
					if !f.atLineStart {
						f.newline()
					}
					f.writeIndent()
					f.writeString(trimmed)
				}
			}
			// Signal continuation if last line has content and next sibling is inline
			lastTrimmed := strings.TrimSpace(lines[len(lines)-1])
			if lastTrimmed != "" {
				if next := nextNonWhitespace(children, child); next != nil && isInlineNode(next) {
					continueInline = true
				}
			}
			lastSignificant = child
			continue
		}

		// All other children: ensure new line with indent.
		if continueInline && isInlineNode(child) {
			f.writeString(" ")
			continueInline = false
		} else {
			continueInline = false
			if !f.atLineStart {
				f.newline()
			}
			f.writeIndent()
		}
		addLeft := trimFirst != nil && child == trimFirst
		addRight := trimLast != nil && child == trimLast
		if addLeft || addRight {
			f.nodeWithBoundaryTrim(child, addLeft, addRight)
		} else {
			f.node(child)
		}

		lastSignificant = child
	}
}

// nodeWithBoundaryTrim writes a node with overridden trim markers.
// For action and template nodes, it forces trim on the specified sides.
// For other node types, it falls back to normal formatting.
func (f *formatter) nodeWithBoundaryTrim(n template.Node, trimLeft, trimRight bool) {
	switch n := n.(type) {
	case *template.ActionNode:
		f.writeString(f.leftDelim)
		if n.TrimLeft || trimLeft {
			f.writeString("- ")
		}
		f.writePipe(n.Pipe)
		if n.TrimRight || trimRight {
			f.writeString(" -")
		}
		f.writeString(f.rightDelim)
	case *template.TemplateNode:
		f.writeString(f.leftDelim)
		if n.TrimLeft || trimLeft {
			f.writeString("- ")
		}
		f.writeString("template ")
		f.writeString(strconv.Quote(n.Name))
		if n.Pipe != nil {
			f.sb.WriteByte(' ')
			f.writePipe(n.Pipe)
		}
		if n.TrimRight || trimRight {
			f.writeString(" -")
		}
		f.writeString(f.rightDelim)
	default:
		f.node(n)
	}
}

// significantBoundaries returns the first and last non-whitespace children.
func significantBoundaries(children []template.Node) (first, last template.Node) {
	for _, child := range children {
		if tn, ok := child.(*template.TextNode); ok && strings.TrimSpace(string(tn.Text)) == "" {
			continue
		}
		if first == nil {
			first = child
		}
		last = child
	}
	return
}

// formatChildrenInline formats children without adding block-level whitespace.
// Leading and trailing whitespace-only text nodes are stripped — they are
// formatting artifacts from block layout, not meaningful content.
func (f *formatter) formatChildrenInline(children []template.Node) {
	start := 0
	for start < len(children) {
		if tn, ok := children[start].(*template.TextNode); ok && strings.TrimSpace(string(tn.Text)) == "" {
			start++
		} else {
			break
		}
	}
	end := len(children)
	for end > start {
		if tn, ok := children[end-1].(*template.TextNode); ok && strings.TrimSpace(string(tn.Text)) == "" {
			end--
		} else {
			break
		}
	}
	for _, child := range children[start:end] {
		f.nodeInline(child)
	}
}

// nodeInline formats a node inline (no block-level indentation decisions).
func (f *formatter) nodeInline(n template.Node) {
	if n == nil {
		return
	}
	switch n := n.(type) {
	case *template.TextNode:
		text := string(n.Text)
		if strings.TrimSpace(text) == "" {
			if text != "" {
				f.writeString(" ")
			}
			return
		}
		if strings.Contains(text, "\n") {
			// Collapse newlines but preserve meaningful leading/trailing spaces.
			// A leading space separates from a preceding inline sibling;
			// a trailing space separates from a following sibling.
			// Newlines and tabs at boundaries are formatting artifacts.
			hasLeadingSpace := text[0] == ' '
			hasTrailingSpace := text[len(text)-1] == ' '
			text = strings.TrimSpace(text)
			if hasLeadingSpace {
				text = " " + text
			}
			if hasTrailingSpace {
				text = text + " "
			}
		}
		f.writeString(text)
	case *template.HTMLElementNode:
		f.writeString("<")
		f.writeString(n.TagName)
		for _, attr := range n.Attr {
			f.sb.WriteByte(' ')
			f.writeString(attr.String())
		}
		if n.Void {
			f.writeString(">")
			return
		}
		f.writeString(">")
		f.formatChildrenInline(n.Children)
		f.writeString("</")
		f.writeString(n.TagName)
		f.writeString(">")
	case *template.ActionNode:
		f.actionNode(n)
	case *template.TemplateNode:
		f.templateNode(n)
	case *template.IfNode:
		// Inline branch in text — use original source
		f.writeString(n.String())
	case *template.RangeNode:
		f.writeString(n.String())
	case *template.WithNode:
		f.writeString(n.String())
	case *template.CommentNode:
		f.commentNode(n)
	default:
		f.node(n)
	}
}

// childrenHaveLineBreaks returns true if any child contains a newline,
// indicating source-level line breaks. Checks text nodes directly and
// other nodes via their String() representation.
func childrenHaveLineBreaks(children []template.Node) bool {
	for _, child := range children {
		if strings.Contains(child.String(), "\n") {
			return true
		}
	}
	return false
}

// allChildrenInline returns true when every child is an inline-flow node
// (text, inline HTML elements, actions, template calls).
func allChildrenInline(children []template.Node) bool {
	for _, child := range children {
		if !isInlineNode(child) {
			return false
		}
	}
	return true
}

// estimateElementInlineLength estimates the rendered length of an element
// if it were formatted inline (open tag + children + close tag).
func estimateElementInlineLength(n *template.HTMLElementNode) int {
	// <tag attrs>children</tag>
	length := 1 + len(n.TagName) // "<tag"
	for _, attr := range n.Attr {
		length += 1 + len(attr.String()) // " attr"
	}
	length++ // ">"
	for _, child := range n.Children {
		length += len(child.String())
	}
	if !n.Void {
		length += 3 + len(n.TagName) // "</tag>"
	}
	return length
}

// nextNonWhitespace returns the next non-whitespace-only sibling after current
// in children, or nil if none exists.
func nextNonWhitespace(children []template.Node, current template.Node) template.Node {
	found := false
	for _, child := range children {
		if child == current {
			found = true
			continue
		}
		if !found {
			continue
		}
		if tn, ok := child.(*template.TextNode); ok {
			if strings.TrimSpace(string(tn.Text)) == "" {
				continue
			}
		}
		return child
	}
	return nil
}

// isInlineNode returns true for nodes that participate in inline text flow:
// text nodes, inline HTML elements, actions, and template calls.
func isInlineNode(n template.Node) bool {
	switch n := n.(type) {
	case *template.TextNode:
		return true
	case *template.HTMLElementNode:
		return inlineElements[strings.ToLower(n.TagName)]
	case *template.ActionNode, *template.TemplateNode:
		return true
	}
	return false
}

func isVoidHTMLElement(n template.Node) bool {
	e, ok := n.(*template.HTMLElementNode)
	return ok && e.Void
}

// canInlineBody returns true when a list of nodes can be formatted inline:
// no meaningful line breaks and total content length within maxLineWidth.
// Whitespace-only text nodes are ignored (they represent source formatting).
// Text nodes with content are checked after trimming leading/trailing whitespace.
func (f *formatter) canInlineBody(nodes []template.Node) bool {
	length := 0
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.TextNode:
			text := strings.TrimSpace(string(n.Text))
			if text == "" {
				if hasBlankLine(string(n.Text)) {
					return false
				}
				continue
			}
			if strings.Contains(text, "\n") {
				return false
			}
			length += len(text)
		default:
			s := n.String()
			if strings.Contains(s, "\n") {
				return false
			}
			length += len(s)
		}
	}
	return length <= f.maxLineWidth
}

// elseChain detects when an ElseList has a single branch child (IfNode or WithNode)
// and returns it for "else if" / "else with" chaining.
func elseChain(elseList *template.ListNode) (*template.BranchNode, string) {
	if elseList == nil || len(elseList.Nodes) != 1 {
		return nil, ""
	}
	switch n := elseList.Nodes[0].(type) {
	case *template.IfNode:
		return &n.BranchNode, "if"
	case *template.WithNode:
		return &n.BranchNode, "with"
	}
	return nil, ""
}

// cleanUnnecessaryTrim removes trim markers from the formatted output when
// there is no whitespace to trim. A left trim marker ({{- ) is unnecessary
// when preceded by non-whitespace or at the start of the string. A right trim
// marker ( -}}) is unnecessary when followed by non-whitespace or at the end
// of the string.
func cleanUnnecessaryTrim(s, leftDelim, rightDelim string) string {
	leftTrim := leftDelim + "- "
	rightTrim := " -" + rightDelim

	// First pass: remove unnecessary left trims.
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+len(leftTrim) <= len(s) && s[i:i+len(leftTrim)] == leftTrim {
			if i == 0 || !isWhitespace(s[i-1]) {
				b.WriteString(leftDelim)
				i += len(leftTrim)
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}

	// Second pass: remove unnecessary right trims.
	s = b.String()
	b.Reset()
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+len(rightTrim) <= len(s) && s[i:i+len(rightTrim)] == rightTrim {
			after := i + len(rightTrim)
			if after >= len(s) || !isWhitespace(s[after]) {
				b.WriteString(rightDelim)
				i += len(rightTrim)
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}

	return b.String()
}

// hasBlankLine reports whether s contains at least one blank line
// (two consecutive newlines, possibly with whitespace between them).
func hasBlankLine(s string) bool {
	lines := strings.Split(s, "\n")
	if len(lines) < 3 {
		return false
	}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) == "" {
			return true
		}
	}
	return false
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// cleanWhitespace performs gofmt-style whitespace normalization:
//  1. Trim trailing whitespace (spaces/tabs) from every line
//  2. Collapse runs of blank lines to at most one
//  3. Skip both above inside whitespace-sensitive elements (pre, textarea, etc.)
//  4. Trim trailing blank lines at the end of output
func cleanWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	preserveDepth := 0
	prevBlank := false

	for _, line := range lines {
		if preserveDepth > 0 {
			// Inside whitespace-sensitive element: pass through unchanged
			preserveDepth += countSensitiveOpens(line) - countSensitiveCloses(line)
			out = append(out, line)
			prevBlank = false
			continue
		}

		opens := countSensitiveOpens(line)
		closes := countSensitiveCloses(line)
		if opens > closes {
			// Line opens a sensitive element — pass through, enter preserve mode
			preserveDepth = opens - closes
			out = append(out, strings.TrimRight(line, " \t"))
			prevBlank = false
			continue
		}

		line = strings.TrimRight(line, " \t")
		blank := line == ""

		if blank && prevBlank {
			continue
		}

		out = append(out, line)
		prevBlank = blank
	}

	// Trim trailing blank lines
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	return strings.Join(out, "\n")
}

// stripQuotedValues removes content inside quoted attribute values from the
// line so that tag-like text inside attribute values (e.g., <div title="<pre>">)
// is not mistakenly matched as actual HTML tags.
func stripQuotedValues(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	inQuote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
				b.WriteByte(c)
			}
			// Skip content inside quotes.
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
		}
		b.WriteByte(c)
	}
	return b.String()
}

func countSensitiveOpens(line string) int {
	count := 0
	lower := strings.ToLower(stripQuotedValues(line))
	for tag := range whitespaceSensitiveElements {
		// Match opening tags: <pre, <textarea, etc.
		search := "<" + tag
		i := 0
		for i < len(lower) {
			idx := strings.Index(lower[i:], search)
			if idx < 0 {
				break
			}
			pos := i + idx + len(search)
			if pos >= len(lower) || lower[pos] == ' ' || lower[pos] == '>' || lower[pos] == '\t' || lower[pos] == '\n' || lower[pos] == '/' {
				count++
			}
			i = pos
		}
	}
	return count
}

func countSensitiveCloses(line string) int {
	count := 0
	lower := strings.ToLower(stripQuotedValues(line))
	for tag := range whitespaceSensitiveElements {
		search := "</" + tag
		i := 0
		for i < len(lower) {
			idx := strings.Index(lower[i:], search)
			if idx < 0 {
				break
			}
			pos := i + idx + len(search)
			if pos >= len(lower) || lower[pos] == '>' || lower[pos] == ' ' || lower[pos] == '\t' {
				count++
			}
			i = pos
		}
	}
	return count
}

// Source formats a Go HTML template file. The file's final whitespace run is
// left exactly as written: Source never adds or removes a trailing newline.
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
	out := Node(tree.Root, opts...)
	return strings.TrimRight(out, whitespace) + trailingWhitespace(src), nil
}

const whitespace = " \t\r\n"

func trailingWhitespace(s string) string {
	return s[len(strings.TrimRight(s, whitespace)):]
}
