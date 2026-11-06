package template

import (
	"slices"
	"strings"
)

// HTMLContext represents the escaping context at a template action node.
// This mirrors the key fields of html/template's context type.
type HTMLContext struct {
	State   string // "text", "attr", "comment"
	Element string // element name (e.g., "script", "style", "p") or ""
	Attr    string // attribute name (e.g., "href", "onclick", "class") or ""
	// JSContext is set when State is "text" and Element is "script". It is
	// "expr" outside of any string/template literal, "str" inside ' or "
	// strings, "tpl" inside backtick template literals, "regex" inside a
	// regex literal, or "comment" inside a JS comment.
	//
	// A ${...} interpolation inside a template literal is an expression
	// position, so it reports "expr", not "tpl".
	JSContext string
}

// AnnotateHTMLContexts walks the tree and sets HTMLContext on each ActionNode
// based on its structural position in the AST.
func AnnotateHTMLContexts(root Node) {
	annotateContexts(root, "", "", false, "")
}

func annotateContexts(n Node, parentElement, parentAttr string, inComment bool, jsCtx string) {
	if n == nil {
		return
	}
	switch n := n.(type) {
	case *ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			annotateContexts(child, "", "", false, "")
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			annotateContexts(attr, n.TagName, "", false, "")
		}
		if strings.EqualFold(n.TagName, "script") {
			annotateScriptChildren(n.Children)
			return
		}
		for _, child := range n.Children {
			annotateContexts(child, n.TagName, "", false, "")
		}
	case *HTMLAttributeNode:
		attrName := ""
		if len(n.Key) > 0 {
			if tn, ok := n.Key[0].(*TextNode); ok {
				attrName = strings.ToLower(string(tn.Text))
			}
		}
		for _, v := range n.Value {
			annotateContexts(v, parentElement, attrName, false, "")
		}
	case *HTMLCommentNode:
		for _, child := range n.Children {
			annotateContexts(child, "", "", true, "")
		}
	case *ActionNode:
		if inComment {
			n.HTMLContext = &HTMLContext{State: "comment"}
		} else {
			state := "text"
			if parentAttr != "" {
				state = "attr"
			}
			n.HTMLContext = &HTMLContext{
				State:     state,
				Element:   strings.ToLower(parentElement),
				Attr:      parentAttr,
				JSContext: jsCtx,
			}
		}
	case *TemplateNode:
		if inComment {
			n.HTMLContext = &HTMLContext{State: "comment"}
		} else {
			state := "text"
			if parentAttr != "" {
				state = "attr"
			}
			n.HTMLContext = &HTMLContext{
				State:     state,
				Element:   strings.ToLower(parentElement),
				Attr:      parentAttr,
				JSContext: jsCtx,
			}
		}
	case *IfNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *RangeNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *WithNode:
		annotateBranch(&n.BranchNode, parentElement, parentAttr, inComment, jsCtx)
	case *DefineNode:
		if n.Body != nil {
			for _, child := range n.Body.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	case *BlockNode:
		if n.Body != nil {
			for _, child := range n.Body.Nodes {
				annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
			}
		}
	}
}

func annotateBranch(n *BranchNode, parentElement, parentAttr string, inComment bool, jsCtx string) {
	if n.List != nil {
		for _, child := range n.List.Nodes {
			annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
		}
	}
	if n.ElseList != nil {
		for _, child := range n.ElseList.Nodes {
			annotateContexts(child, parentElement, parentAttr, inComment, jsCtx)
		}
	}
}

// annotateScriptChildren walks the children of a <script> element in source
// order, scanning the text content as JavaScript to determine the context
// (expression, string, template literal, comment) at each action position.
//
// The scanner is intentionally simple: it tracks string, template-literal
// and comment delimiters, plus the brace depth of any ${...} interpolation
// so it can tell a string position from the expression position inside an
// interpolation. It does not otherwise interpret JS syntax. When the state
// can't be determined (a control structure entered an unbalanced state),
// the scanner conservatively reports "expr" so the value gets the stronger
// jsValEscape treatment.
func annotateScriptChildren(children []Node) {
	state := scriptScanState{ctx: "expr"}
	for _, child := range children {
		state = annotateScriptNode(child, state)
	}
}

type scriptScanState struct {
	ctx   string // "expr", "str", "tpl", "regex", "lcomment", "bcomment"
	quote byte   // for "str": '\'' or '"'
	// braceDepth holds one entry per enclosing ${...} interpolation,
	// counting the unmatched '{' seen inside it. A '}' that would take
	// the innermost entry below zero closes that interpolation and
	// returns the scanner to the surrounding template literal. An empty
	// stack means the scanner is not inside any interpolation.
	braceDepth []int
}

// clone returns a copy of s that shares no brace accounting with s.
// Branch bodies are scanned from the state at branch entry, so they must
// not write through to the enclosing scan's stack.
func (s scriptScanState) clone() scriptScanState {
	s.braceDepth = slices.Clone(s.braceDepth)
	return s
}

func annotateScriptNode(n Node, state scriptScanState) scriptScanState {
	switch n := n.(type) {
	case *TextNode:
		return scanScriptText(string(n.Text), state)
	case *ActionNode:
		ctx := state.ctx
		if ctx == "lcomment" || ctx == "bcomment" {
			ctx = "comment"
		}
		n.HTMLContext = &HTMLContext{
			State:     "text",
			Element:   "script",
			JSContext: ctx,
		}
		return state
	case *TemplateNode:
		ctx := state.ctx
		if ctx == "lcomment" || ctx == "bcomment" {
			ctx = "comment"
		}
		n.HTMLContext = &HTMLContext{
			State:     "text",
			Element:   "script",
			JSContext: ctx,
		}
		return state
	case *IfNode:
		// Template branches inside <script> are tricky: a branch can
		// open a string or an interpolation and the matching else can
		// close one. The safest option is to annotate each branch
		// independently from the state at branch entry, then return the
		// entry state — both branches must produce code that's
		// syntactically balanced from the reader's perspective.
		annotateScriptBranch(&n.BranchNode, state)
		return state
	case *RangeNode:
		annotateScriptBranch(&n.BranchNode, state)
		return state
	case *WithNode:
		annotateScriptBranch(&n.BranchNode, state)
		return state
	}
	return state
}

func annotateScriptBranch(b *BranchNode, state scriptScanState) {
	if b.List != nil {
		s := state.clone()
		for _, child := range b.List.Nodes {
			s = annotateScriptNode(child, s)
		}
	}
	if b.ElseList != nil {
		s := state.clone()
		for _, child := range b.ElseList.Nodes {
			s = annotateScriptNode(child, s)
		}
	}
}

func scanScriptText(s string, state scriptScanState) scriptScanState {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state.ctx {
		case "expr", "regex":
			switch c {
			case '\'', '"':
				state.ctx = "str"
				state.quote = c
			case '`':
				state.ctx = "tpl"
			case '/':
				if i+1 < len(s) {
					switch s[i+1] {
					case '/':
						state.ctx = "lcomment"
						i++
						continue
					case '*':
						state.ctx = "bcomment"
						i++
						continue
					}
				}
				// Otherwise leave as-is: distinguishing regex from
				// division requires lookback into JS tokens. The
				// caller treats the default ("expr") as the safer
				// option, so leave state alone.
			case '{':
				if len(state.braceDepth) > 0 {
					state.braceDepth[len(state.braceDepth)-1]++
				}
			case '}':
				if len(state.braceDepth) > 0 {
					last := len(state.braceDepth) - 1
					state.braceDepth[last]--
					if state.braceDepth[last] < 0 {
						state.braceDepth = state.braceDepth[:last]
						state.ctx = "tpl"
					}
				}
			}
		case "str":
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
				}
			case state.quote:
				state.ctx = "expr"
				state.quote = 0
			case '\n':
				// Unterminated string. Reset to expr so the next
				// action gets jsValEscape conservatively.
				state.ctx = "expr"
				state.quote = 0
			}
		case "tpl":
			switch c {
			case '\\':
				if i+1 < len(s) {
					i++
				}
			case '$':
				// "${" opens an interpolation, which is an expression
				// position: code there runs.
				if i+1 < len(s) && s[i+1] == '{' {
					state.braceDepth = append(state.braceDepth, 0)
					state.ctx = "expr"
					i++
				}
			case '`':
				state.ctx = "expr"
			}
		case "lcomment":
			if c == '\n' {
				state.ctx = "expr"
			}
		case "bcomment":
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				state.ctx = "expr"
				i++
			}
		}
	}
	return state
}
