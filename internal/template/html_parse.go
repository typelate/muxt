package template

import (
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template/lex"
)

// voidElements is the set of HTML elements that have no closing tag.
var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}

// rawTextElements is the set of HTML elements whose content is raw text
// (no child elements, but template actions are still recognized).
var rawTextElements = map[string]bool{
	"script":   true,
	"style":    true,
	"textarea": true,
	"title":    true,
}

// FormattingElements is the WHATWG §13.2.4.2 set subject to the
// adoption agency algorithm (§13.2.6.4.7).
var FormattingElements = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true,
	"font": true, "i": true, "nobr": true, "s": true, "small": true,
	"strike": true, "strong": true, "tt": true, "u": true,
}

// ImpliedEndTagElements is the WHATWG §13.2.6.3 "generate implied end
// tags" set plus the "thoroughly" additions: elements whose end tags
// authors may legally omit.
var ImpliedEndTagElements = map[string]bool{
	"dd": true, "dt": true, "li": true, "optgroup": true, "option": true,
	"p": true, "rb": true, "rp": true, "rt": true, "rtc": true,
	"caption": true, "colgroup": true, "tbody": true, "td": true,
	"tfoot": true, "th": true, "thead": true, "tr": true,
}

// htmlParse is the top-level parser for HTML+template content.
// It produces an HTMLDocumentFragmentNode containing the parsed content.
func (t *Tree) htmlParse() error {
	frag := t.newHTMLDocumentFragment(t.peek().Pos)
	for t.peek().Type != lex.TokenTypeEOF {
		n, err := t.htmlParseNode()
		if err != nil {
			return err
		}
		switch n.(type) {
		case *endNode, *elseNode:
			return t.errorf("unexpected %s", n)
		}
		if n != nil {
			frag.Children = append(frag.Children, n)
		}
	}
	frag.end = lex.Pos(len(t.text))
	// Store the fragment as the tree root by wrapping in a ListNode.
	t.Root = t.newList(0)
	var n Node = frag
	t.Root.Nodes = append(t.Root.Nodes, n)
	t.Root.end = lex.Pos(len(t.text))
	return nil
}

// htmlParseNode dispatches parsing based on the current token type.
// This handles both top-level HTML nodes and attribute tokens that appear
// inside template control structure bodies (e.g., {{if .Show}}class="x"{{end}}
// in an attribute position).
func (t *Tree) htmlParseNode() (Node, error) {
	switch token := t.next(); token.Type {
	case lex.TokenTypeHTMLText, lex.TokenTypeHTMLAttrValue:
		return t.newText(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeHTMLTagOpen:
		return t.htmlParseStartTag(token)
	case lex.TokenTypeHTMLEndTagOpen:
		return t.htmlParseEndTag(token)
	case lex.TokenTypeHTMLCommentOpen:
		return t.htmlParseComment(token)
	case lex.TokenTypeHTMLDoctype:
		return t.newHTMLDoctype(token.Pos, token.End, token.Value), nil
	case lex.TokenTypeHTMLAttrName:
		// Attribute token inside a template control structure body
		// (e.g., {{if .Show}}class="visible"{{end}} in attribute position).
		attr, err := t.htmlParseAttribute(token)
		if err != nil {
			return nil, err
		}
		return attr, nil
	case lex.TokenTypeComment:
		c := t.newComment(token.Pos, token.End, token.Value)
		c.TrimLeft = token.TrimLeft
		c.TrimRight = token.TrimRight
		return c, nil
	case lex.TokenTypeLeftDelim:
		t.actionLine = token.Line
		t.actionPos = token.Pos
		t.actionTrimLeft = token.TrimLeft
		defer t.clearActionLine()
		return t.htmlParseTemplateAction()
	case lex.TokenTypeEOF:
		t.backup()
		return nil, nil
	default:
		return nil, t.unexpected(token, "HTML input")
	}
}

// htmlParseStartTag parses an HTML start tag after '<' has been consumed.
// Returns an HTMLElementNode.
func (t *Tree) htmlParseStartTag(openToken lex.Token) (Node, error) {
	nameToken := t.next()
	if nameToken.Type != lex.TokenTypeHTMLStartTag {
		return nil, t.unexpected(nameToken, "HTML start tag name")
	}
	tagName := nameToken.Value
	elem := t.newHTMLElement(openToken.Pos, tagName)
	elem.AttrStart = nameToken.End

	// ParseTree attributes and tag close.
	if err := t.htmlParseAttributes(elem); err != nil {
		return nil, err
	}

	// Check if this is a void element.
	if voidElements[strings.ToLower(tagName)] {
		elem.Void = true
		return elem, nil
	}
	if elem.Void {
		// Self-closing tag (set by htmlParseAttributes when it sees />).
		return elem, nil
	}

	// Check if this is a raw text element.
	if rawTextElements[strings.ToLower(tagName)] {
		if err := t.htmlParseRawTextContent(elem); err != nil {
			return nil, err
		}
		return elem, nil
	}

	// ParseTree children until matching end tag.
	t.htmlAncestors = append(t.htmlAncestors, strings.ToLower(tagName))
	err := t.htmlParseChildren(elem)
	t.htmlAncestors = t.htmlAncestors[:len(t.htmlAncestors)-1]
	if err != nil {
		return nil, err
	}
	return elem, nil
}

// htmlParseAttributes parses attributes until '>' or '/>' is found.
func (t *Tree) htmlParseAttributes(elem *HTMLElementNode) error {
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLTagEnd:
			elem.AttrEnd = token.End
			elem.end = token.End
			return nil
		case lex.TokenTypeHTMLSelfClose:
			elem.Void = true
			elem.AttrEnd = token.End
			elem.end = token.End
			return nil
		case lex.TokenTypeHTMLAttrName:
			attr, err := t.htmlParseAttribute(token)
			if err != nil {
				return err
			}
			elem.Attr = append(elem.Attr, attr)
		case lex.TokenTypeLeftDelim:
			// Template action in attribute position (e.g., {{if .Show}}class="x"{{end}}).
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return err
			}
			if action != nil {
				elem.Attr = append(elem.Attr, action)
			}
		case lex.TokenTypeEOF:
			return t.errorf("unclosed HTML tag %q", elem.TagName)
		default:
			return t.unexpected(token, "HTML tag attributes")
		}
	}
}

// htmlParseAttribute parses a single attribute. The attribute name token has
// already been consumed.
func (t *Tree) htmlParseAttribute(nameToken lex.Token) (*HTMLAttributeNode, error) {
	attr := t.newHTMLAttribute(nameToken.Pos)
	attr.Key = append(attr.Key, t.newText(nameToken.Pos, nameToken.End, nameToken.Value))
	attr.end = nameToken.End

	// Check for '='.
	next := t.next()
	if next.Type != lex.TokenTypeHTMLAttrEquals {
		// Boolean attribute — no value.
		t.backup()
		return attr, nil
	}
	attr.HasValue = true

	// ParseTree value parts (may be multiple tokens with template actions interleaved).
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLAttrValue:
			attr.Value = append(attr.Value, t.newText(token.Pos, token.End, token.Value))
			attr.end = token.End
		case lex.TokenTypeLeftDelim:
			// Check if this is {{end}} or {{else}} — if so, it terminates
			// a control structure body, not the attribute value.
			next := t.nextNonSpace()
			if next.Type == lex.TokenTypeEnd || next.Type == lex.TokenTypeElse {
				// Back up both the peeked token and the left delimiter
				// so the caller (itemList) can process {{end}}/{{else}}.
				t.backup2(token)
				return attr, nil
			}
			// Not a control terminator — parse as template action in value.
			t.backup()
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return nil, err
			}
			if action != nil {
				attr.Value = append(attr.Value, action)
				attr.end = action.End()
			}
		default:
			// End of value — this token belongs to the next attribute or tag close.
			t.backup()
			return attr, nil
		}
	}
}

// htmlParseComment parses an HTML comment after '<!--' has been consumed.
func (t *Tree) htmlParseComment(openToken lex.Token) (Node, error) {
	comment := t.newHTMLComment(openToken.Pos)
	for {
		token := t.next()
		switch token.Type {
		case lex.TokenTypeHTMLComment:
			comment.Children = append(comment.Children, t.newText(token.Pos, token.End, token.Value))
		case lex.TokenTypeHTMLCommentClose:
			comment.end = token.End
			return comment, nil
		case lex.TokenTypeLeftDelim:
			// Template action inside comment.
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return nil, err
			}
			if action != nil {
				comment.Children = append(comment.Children, action)
			}
		case lex.TokenTypeEOF:
			return nil, t.errorf("unclosed HTML comment")
		default:
			return nil, t.unexpected(token, "HTML comment")
		}
	}
}

// htmlParseEndTag parses an end tag after '</' has been consumed.
// Returns the end tag as a text node since the parser doesn't match it
// structurally here — that's done by the parent element parser.
func (t *Tree) htmlParseEndTag(openToken lex.Token) (Node, error) {
	// This is called when we encounter </tag> at the top level of the fragment
	// or when it doesn't match the expected parent. Just consume it.
	nameToken := t.next()
	if nameToken.Type != lex.TokenTypeHTMLEndTag {
		return nil, t.unexpected(nameToken, "HTML end tag name")
	}
	closeToken := t.next()
	if closeToken.Type != lex.TokenTypeHTMLTagEnd {
		return nil, t.unexpected(closeToken, "HTML end tag close")
	}
	// Browsers ignore this stray end tag; record a diagnostic and
	// preserve it verbatim (byte-parity with stdlib html/template).
	t.diag(openToken.Pos, closeToken.End, SeverityWarning, DiagOrphanEndTag, msgOrphanEndTag, nameToken.Value)
	text := t.newText(openToken.Pos, closeToken.End, "</"+nameToken.Value+">")
	text.Verbatim = true
	return text, nil
}

// htmlParseChildren parses child nodes of an element until the matching
// end tag is found.
func (t *Tree) htmlParseChildren(elem *HTMLElementNode) error {
	t.htmlDepth++
	defer func() { t.htmlDepth-- }()
	if t.htmlDepth > t.maxHTMLDepth {
		return t.errorf("maximum HTML element nesting depth exceeded (%d)", t.maxHTMLDepth)
	}
	for {
		token := t.peek()
		switch token.Type {
		case lex.TokenTypeHTMLEndTagOpen:
			// Check if this is our matching end tag.
			endTagOpen := t.next() // consume </
			nameToken := t.next()
			if nameToken.Type == lex.TokenTypeHTMLEndTag && strings.EqualFold(nameToken.Value, elem.TagName) {
				// Matching end tag — consume '>' and return.
				closeToken := t.next()
				if closeToken.Type != lex.TokenTypeHTMLTagEnd {
					return t.unexpected(closeToken, "HTML end tag close")
				}
				elem.Closed = true
				elem.EndTagStart = endTagOpen.Pos
				elem.end = closeToken.End
				return nil
			}
			// Not our end tag. Check if it matches an open ancestor element.
			// If so, implicitly close this element and back up so the
			// ancestor can consume its closing tag (like x/net/html).
			if nameToken.Type == lex.TokenTypeHTMLEndTag && t.htmlHasAncestor(nameToken.Value) {
				tag := strings.ToLower(elem.TagName)
				if FormattingElements[tag] {
					t.diag(endTagOpen.Pos, nameToken.End, SeverityWarning, DiagMisnestedTag,
						msgMisnestedClose, nameToken.Value, elem.TagName, elem.TagName)
				}
				t.backup2(token)
				return nil
			}
			// No ancestor match — orphan end tag. Browsers ignore it;
			// record a diagnostic and preserve it verbatim as a text
			// child (byte-parity with stdlib html/template).
			if nameToken.Type == lex.TokenTypeHTMLEndTag {
				closeToken := t.next() // consume >
				t.diag(endTagOpen.Pos, closeToken.End, SeverityWarning, DiagOrphanEndTag, msgOrphanEndTag, nameToken.Value)
				text := t.newText(endTagOpen.Pos, closeToken.End, "</"+nameToken.Value+">")
				text.Verbatim = true
				elem.Children = append(elem.Children, text)
			} else {
				// nameToken is not an end tag name (e.g., lexer error).
				// Back up so the next iteration can handle it properly.
				return t.unexpected(nameToken, "HTML end tag name")
			}
		case lex.TokenTypeLeftDelim:
			// Peek ahead to see if this is {{end}} or {{else}} which would
			// terminate an enclosing control structure. If so, leave the
			// element unclosed and return so the control structure can
			// process the terminator.
			if t.htmlPeekIsControlTerminator() {
				return nil
			}
			n, err := t.htmlParseNode()
			if err != nil {
				return err
			}
			if n != nil {
				elem.Children = append(elem.Children, n)
			}
		case lex.TokenTypeEOF:
			// Unclosed element — don't error, just return what we have.
			// Structural diagnostics would flag this.
			return nil
		default:
			n, err := t.htmlParseNode()
			if err != nil {
				return err
			}
			if n != nil {
				elem.Children = append(elem.Children, n)
			}
		}
	}
}

// htmlParseRawTextContent parses the content of raw text elements (script, style, etc.).
// The lexer emits raw text content as TokenTypeHTMLText (stopping only at {{
// and the matching </tagname>), so this is a straightforward token loop.
func (t *Tree) htmlParseRawTextContent(elem *HTMLElementNode) error {
	for {
		token := t.peek()
		switch token.Type {
		case lex.TokenTypeHTMLText:
			t.next()
			elem.Children = append(elem.Children, t.newText(token.Pos, token.End, token.Value))
		case lex.TokenTypeHTMLEndTagOpen:
			// Consume matching end tag tokens.
			endTagOpen := t.next() // consume </
			nameToken := t.next()
			if nameToken.Type != lex.TokenTypeHTMLEndTag || !strings.EqualFold(nameToken.Value, elem.TagName) {
				return t.unexpected(nameToken, "raw text element end tag")
			}
			closeToken := t.next()
			if closeToken.Type != lex.TokenTypeHTMLTagEnd {
				return t.unexpected(closeToken, "raw text element end tag close")
			}
			elem.Closed = true
			elem.EndTagStart = endTagOpen.Pos
			elem.end = closeToken.End
			return nil
		case lex.TokenTypeLeftDelim:
			t.next()
			t.actionLine = token.Line
			t.actionPos = token.Pos
			t.actionTrimLeft = token.TrimLeft
			action, err := t.htmlParseTemplateAction()
			t.clearActionLine()
			if err != nil {
				return err
			}
			if action != nil {
				elem.Children = append(elem.Children, action)
			}
		case lex.TokenTypeEOF:
			return nil
		default:
			return t.unexpected(t.next(), "raw text element content")
		}
	}
}

// htmlPeekIsControlTerminator peeks ahead without consuming tokens to check
// if the current position starts a {{end}} or {{else}} action. This is used
// by htmlParseChildren to stop consuming tokens when a template control
// terminator is encountered, allowing the enclosing itemList to process it.
func (t *Tree) htmlPeekIsControlTerminator() bool {
	// Current peek is LeftDelim. Consume it and the next non-space token
	// to check the keyword, then back them up.
	leftDelim := t.next() // consume {{
	keyword := t.nextNonSpace()
	isTerminator := keyword.Type == lex.TokenTypeEnd || keyword.Type == lex.TokenTypeElse
	t.backup2(leftDelim)
	return isTerminator
}

// htmlHasAncestor reports whether tag (case-insensitive) matches any
// element on the open-element ancestor stack. This is used to decide
// whether a mismatched end tag should implicitly close the current element
// (when the tag belongs to an ancestor) or be treated as an orphan
// (preserved verbatim, diagnosed, not structural).
func (t *Tree) htmlHasAncestor(tag string) bool {
	return slices.Contains(t.htmlAncestors, strings.ToLower(tag))
}

// htmlParseTemplateAction parses a template action (the left delimiter has
// been consumed). This dispatches to the existing template action parser.
func (t *Tree) htmlParseTemplateAction() (Node, error) {
	switch token := t.nextNonSpace(); token.Type {
	case lex.TokenTypeBlock:
		return t.blockControl()
	case lex.TokenTypeBreak:
		return t.breakControl(t.actionPos, token.Line)
	case lex.TokenTypeContinue:
		return t.continueControl(t.actionPos, token.Line)
	case lex.TokenTypeDefine:
		return t.htmlDefineControl(token)
	case lex.TokenTypeElse:
		return t.elseControl()
	case lex.TokenTypeEnd:
		return t.endControl()
	case lex.TokenTypeIf:
		return t.ifControl()
	case lex.TokenTypeRange:
		return t.rangeControl()
	case lex.TokenTypeTemplate:
		return t.templateControl()
	case lex.TokenTypeWith:
		return t.withControl()
	default:
		t.backup()
		token := t.peek()
		pipe, err := t.pipeline("command", lex.TokenTypeRightDelim)
		if err != nil {
			return nil, err
		}
		a := t.newAction(t.actionPos, pipe.end, token.Line, pipe)
		a.TrimLeft = t.actionTrimLeft
		a.TrimRight = t.actionTrimRight
		return a, nil
	}
}

// htmlDefineControl parses a {{define "name"}}...{{end}} and returns a
// DefineNode that keeps the define inline in the AST. The "define" keyword
// has already been consumed. The openToken is the left delimiter.
func (t *Tree) htmlDefineControl(openToken lex.Token) (Node, error) {
	const context = "define clause"
	// actionPos must be captured now, before the body is parsed: itemList
	// recurses into textOrAction for every nested action, and each of
	// those defers clearActionLine, which zeroes t.actionPos when it
	// returns. Reading t.actionPos after t.itemList() below (as opposed
	// to this saved copy) would always observe 0, mirroring the local
	// capture blockControl already does for the same reason.
	actionPos := t.actionPos
	startTrimLeft := t.actionTrimLeft
	nameToken, err := t.expectOneOf(lex.TokenTypeString, lex.TokenTypeRawString, context)
	if err != nil {
		return nil, err
	}
	name, err := strconv.Unquote(nameToken.Value)
	if err != nil {
		return nil, t.error(err)
	}
	rightDelim, err := t.expect(lex.TokenTypeRightDelim, context)
	if err != nil {
		return nil, err
	}
	startTrimRight := rightDelim.TrimRight
	var end Node
	var body *ListNode
	body, end, err = t.itemList()
	if err != nil {
		return nil, err
	}
	en, ok := end.(*endNode)
	if !ok {
		return nil, t.errorf("unexpected %s in %s", end, context)
	}
	d := t.newDefine(actionPos, end.End(), openToken.Line, name, body)
	d.StartTrimLeft = startTrimLeft
	d.StartTrimRight = startTrimRight
	d.EndTrimLeft = en.TrimLeft
	d.EndTrimRight = en.TrimRight
	return d, nil
}

// htmlTextOrAction handles HTML tokens that appear inside template control
// structure bodies (e.g., inside {{range ...}}...{{end}}).
func (t *Tree) htmlTextOrAction() (Node, error) {
	return t.htmlParseNode()
}
