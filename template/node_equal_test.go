package template

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// requireASTEqual fails the test with a structural diff if got and want are
// not equal (ignoring position, line, and tree pointers). Skips the check when
// want is nil, so tests can opt out of AST assertions while keeping the same
// case shape.
func requireASTEqual(t *testing.T, got, want Node) {
	t.Helper()
	if want == nil {
		return
	}
	if diff := diffNodes(got, want); diff != "" {
		t.Errorf("AST mismatch:\n%s", diff)
	}
}

// diffNodes compares two nodes structurally, ignoring position (Pos, end),
// line (Line), and tree pointer (tr) fields. It returns an empty string if
// the nodes are equal, or a diff-like description of the differences.
func diffNodes(a, b Node) string {
	var sb strings.Builder
	diffNodesAt(&sb, a, b, "")
	return sb.String()
}

func isNodeNil(n Node) bool {
	return n == nil || reflect.ValueOf(n).IsNil()
}

func diffNodesAt(sb *strings.Builder, a, b Node, indent string) {
	aIsNil := isNodeNil(a)
	bIsNil := isNodeNil(b)
	if aIsNil && bIsNil {
		return
	}
	if aIsNil {
		_, _ = fmt.Fprintf(sb, "%s- <nil>\n", indent)
		printNode(sb, b, indent, "+")
		return
	}
	if bIsNil {
		printNode(sb, a, indent, "-")
		_, _ = fmt.Fprintf(sb, "%s+ <nil>\n", indent)
		return
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		printNode(sb, a, indent, "-")
		printNode(sb, b, indent, "+")
		return
	}
	switch a := a.(type) {
	case *ListNode:
		b := b.(*ListNode)
		diffNodeSlices(sb, a.Nodes, b.Nodes, indent+"  ")
	case *TextNode:
		b := b.(*TextNode)
		if string(a.Text) != string(b.Text) || a.Verbatim != b.Verbatim {
			_, _ = fmt.Fprintf(sb, "%s- TextNode %q Verbatim=%t\n", indent, a.Text, a.Verbatim)
			_, _ = fmt.Fprintf(sb, "%s+ TextNode %q Verbatim=%t\n", indent, b.Text, b.Verbatim)
		}
	case *CommentNode:
		b := b.(*CommentNode)
		if a.Text != b.Text {
			_, _ = fmt.Fprintf(sb, "%s- CommentNode %q\n", indent, a.Text)
			_, _ = fmt.Fprintf(sb, "%s+ CommentNode %q\n", indent, b.Text)
		}
		if a.TrimLeft != b.TrimLeft || a.TrimRight != b.TrimRight {
			_, _ = fmt.Fprintf(sb, "%s  CommentNode.Trim: (%v,%v) vs (%v,%v)\n", indent, a.TrimLeft, a.TrimRight, b.TrimLeft, b.TrimRight)
		}
	case *PipeNode:
		b := b.(*PipeNode)
		if a.IsAssign != b.IsAssign {
			_, _ = fmt.Fprintf(sb, "%s  PipeNode.IsAssign:\n", indent)
			_, _ = fmt.Fprintf(sb, "%s- %v\n", indent, a.IsAssign)
			_, _ = fmt.Fprintf(sb, "%s+ %v\n", indent, b.IsAssign)
		}
		if len(a.Decl) != len(b.Decl) {
			_, _ = fmt.Fprintf(sb, "%s  PipeNode.Decl: len %d vs %d\n", indent, len(a.Decl), len(b.Decl))
		}
		for i := range min(len(a.Decl), len(b.Decl)) {
			diffNodesAt(sb, a.Decl[i], b.Decl[i], indent+"  ")
		}
		if len(a.Commands) != len(b.Commands) {
			_, _ = fmt.Fprintf(sb, "%s  PipeNode.Commands: len %d vs %d\n", indent, len(a.Commands), len(b.Commands))
		}
		for i := range min(len(a.Commands), len(b.Commands)) {
			diffNodesAt(sb, a.Commands[i], b.Commands[i], indent+"  ")
		}
	case *ActionNode:
		bb := b.(*ActionNode)
		if a.TrimLeft != bb.TrimLeft || a.TrimRight != bb.TrimRight {
			_, _ = fmt.Fprintf(sb, "%s  ActionNode.Trim: (%v,%v) vs (%v,%v)\n", indent, a.TrimLeft, a.TrimRight, bb.TrimLeft, bb.TrimRight)
		}
		diffNodesAt(sb, a.Pipe, bb.Pipe, indent+"  ")
	case *CommandNode:
		b := b.(*CommandNode)
		if len(a.Args) != len(b.Args) {
			_, _ = fmt.Fprintf(sb, "%s  CommandNode.Args: len %d vs %d\n", indent, len(a.Args), len(b.Args))
		}
		for i := range min(len(a.Args), len(b.Args)) {
			diffNodesAt(sb, a.Args[i], b.Args[i], indent+"  ")
		}
	case *IdentifierNode:
		b := b.(*IdentifierNode)
		if a.Ident != b.Ident {
			_, _ = fmt.Fprintf(sb, "%s- IdentifierNode %q\n", indent, a.Ident)
			_, _ = fmt.Fprintf(sb, "%s+ IdentifierNode %q\n", indent, b.Ident)
		}
	case *VariableNode:
		b := b.(*VariableNode)
		if !slices.Equal(a.Ident, b.Ident) {
			_, _ = fmt.Fprintf(sb, "%s- VariableNode %v\n", indent, a.Ident)
			_, _ = fmt.Fprintf(sb, "%s+ VariableNode %v\n", indent, b.Ident)
		}
	case *DotNode:
		// type-only comparison
	case *NilNode:
		// type-only comparison
	case *FieldNode:
		b := b.(*FieldNode)
		if !slices.Equal(a.Ident, b.Ident) {
			_, _ = fmt.Fprintf(sb, "%s- FieldNode %v\n", indent, a.Ident)
			_, _ = fmt.Fprintf(sb, "%s+ FieldNode %v\n", indent, b.Ident)
		}
	case *ChainNode:
		b := b.(*ChainNode)
		diffNodesAt(sb, a.Node, b.Node, indent+"  ")
		if !slices.Equal(a.Field, b.Field) {
			_, _ = fmt.Fprintf(sb, "%s- ChainNode.Field %v\n", indent, a.Field)
			_, _ = fmt.Fprintf(sb, "%s+ ChainNode.Field %v\n", indent, b.Field)
		}
	case *BoolNode:
		b := b.(*BoolNode)
		if a.True != b.True {
			_, _ = fmt.Fprintf(sb, "%s- BoolNode %v\n", indent, a.True)
			_, _ = fmt.Fprintf(sb, "%s+ BoolNode %v\n", indent, b.True)
		}
	case *NumberNode:
		b := b.(*NumberNode)
		diffNumber(sb, a, b, indent)
	case *StringNode:
		b := b.(*StringNode)
		if a.Quoted != b.Quoted || a.Text != b.Text {
			_, _ = fmt.Fprintf(sb, "%s- StringNode Quoted=%q Text=%q\n", indent, a.Quoted, a.Text)
			_, _ = fmt.Fprintf(sb, "%s+ StringNode Quoted=%q Text=%q\n", indent, b.Quoted, b.Text)
		}
	case *endNode:
		// type-only
	case *elseNode:
		// type-only
	case *IfNode:
		diffBranch(sb, &a.BranchNode, &b.(*IfNode).BranchNode, indent)
	case *RangeNode:
		diffBranch(sb, &a.BranchNode, &b.(*RangeNode).BranchNode, indent)
	case *WithNode:
		diffBranch(sb, &a.BranchNode, &b.(*WithNode).BranchNode, indent)
	case *BreakNode:
		bb := b.(*BreakNode)
		if a.TrimLeft != bb.TrimLeft || a.TrimRight != bb.TrimRight {
			_, _ = fmt.Fprintf(sb, "%s  BreakNode.Trim: (%v,%v) vs (%v,%v)\n", indent, a.TrimLeft, a.TrimRight, bb.TrimLeft, bb.TrimRight)
		}
	case *ContinueNode:
		bb := b.(*ContinueNode)
		if a.TrimLeft != bb.TrimLeft || a.TrimRight != bb.TrimRight {
			_, _ = fmt.Fprintf(sb, "%s  ContinueNode.Trim: (%v,%v) vs (%v,%v)\n", indent, a.TrimLeft, a.TrimRight, bb.TrimLeft, bb.TrimRight)
		}
	case *TemplateNode:
		b := b.(*TemplateNode)
		if a.Name != b.Name {
			_, _ = fmt.Fprintf(sb, "%s- TemplateNode.Name %q\n", indent, a.Name)
			_, _ = fmt.Fprintf(sb, "%s+ TemplateNode.Name %q\n", indent, b.Name)
		}
		if a.TrimLeft != b.TrimLeft || a.TrimRight != b.TrimRight {
			_, _ = fmt.Fprintf(sb, "%s  TemplateNode.Trim: (%v,%v) vs (%v,%v)\n", indent, a.TrimLeft, a.TrimRight, b.TrimLeft, b.TrimRight)
		}
		diffNodesAt(sb, a.Pipe, b.Pipe, indent+"  ")
	case *HTMLDocumentFragmentNode:
		b := b.(*HTMLDocumentFragmentNode)
		diffNodeSlices(sb, a.Children, b.Children, indent+"  ")
	case *HTMLElementNode:
		b := b.(*HTMLElementNode)
		if a.TagName != b.TagName {
			_, _ = fmt.Fprintf(sb, "%s- HTMLElementNode.TagName %q\n", indent, a.TagName)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLElementNode.TagName %q\n", indent, b.TagName)
		}
		if a.Void != b.Void {
			_, _ = fmt.Fprintf(sb, "%s- HTMLElementNode.Void %v\n", indent, a.Void)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLElementNode.Void %v\n", indent, b.Void)
		}
		if a.Closed != b.Closed {
			_, _ = fmt.Fprintf(sb, "%s- HTMLElementNode.Closed %v\n", indent, a.Closed)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLElementNode.Closed %v\n", indent, b.Closed)
		}
		diffNodeSlices(sb, a.Attr, b.Attr, indent+"  ")
		diffNodeSlices(sb, a.Children, b.Children, indent+"  ")
	case *HTMLAttributeNode:
		b := b.(*HTMLAttributeNode)
		if a.Namespace != b.Namespace {
			_, _ = fmt.Fprintf(sb, "%s- HTMLAttributeNode.Namespace %q\n", indent, a.Namespace)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLAttributeNode.Namespace %q\n", indent, b.Namespace)
		}
		if a.HasValue != b.HasValue {
			_, _ = fmt.Fprintf(sb, "%s- HTMLAttributeNode.HasValue %v\n", indent, a.HasValue)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLAttributeNode.HasValue %v\n", indent, b.HasValue)
		}
		diffNodeSlices(sb, a.Key, b.Key, indent+"  ")
		diffNodeSlices(sb, a.Value, b.Value, indent+"  ")
	case *HTMLCommentNode:
		b := b.(*HTMLCommentNode)
		diffNodeSlices(sb, a.Children, b.Children, indent+"  ")
	case *HTMLDoctypeNode:
		b := b.(*HTMLDoctypeNode)
		if a.Text != b.Text {
			_, _ = fmt.Fprintf(sb, "%s- HTMLDoctypeNode %q\n", indent, a.Text)
			_, _ = fmt.Fprintf(sb, "%s+ HTMLDoctypeNode %q\n", indent, b.Text)
		}
	case *DefineNode:
		b := b.(*DefineNode)
		if a.Name != b.Name {
			_, _ = fmt.Fprintf(sb, "%s- DefineNode.Name %q\n", indent, a.Name)
			_, _ = fmt.Fprintf(sb, "%s+ DefineNode.Name %q\n", indent, b.Name)
		}
		if a.StartTrimLeft != b.StartTrimLeft || a.StartTrimRight != b.StartTrimRight {
			_, _ = fmt.Fprintf(sb, "%s  DefineNode.StartTrim: (%v,%v) vs (%v,%v)\n", indent, a.StartTrimLeft, a.StartTrimRight, b.StartTrimLeft, b.StartTrimRight)
		}
		if a.EndTrimLeft != b.EndTrimLeft || a.EndTrimRight != b.EndTrimRight {
			_, _ = fmt.Fprintf(sb, "%s  DefineNode.EndTrim: (%v,%v) vs (%v,%v)\n", indent, a.EndTrimLeft, a.EndTrimRight, b.EndTrimLeft, b.EndTrimRight)
		}
		diffNodesAt(sb, a.Body, b.Body, indent+"  ")
	case *BlockNode:
		b := b.(*BlockNode)
		if a.Name != b.Name {
			_, _ = fmt.Fprintf(sb, "%s- BlockNode.Name %q\n", indent, a.Name)
			_, _ = fmt.Fprintf(sb, "%s+ BlockNode.Name %q\n", indent, b.Name)
		}
		diffNodesAt(sb, a.Pipe, b.Pipe, indent+"  ")
		diffNodesAt(sb, a.Body, b.Body, indent+"  ")
	default:
		panic(fmt.Sprintf("diffNodes: unhandled node type %T", a))
	}
}

func diffNumber(sb *strings.Builder, a, b *NumberNode, indent string) {
	if a.IsInt != b.IsInt || a.IsUint != b.IsUint || a.IsFloat != b.IsFloat ||
		a.IsComplex != b.IsComplex || a.Int64 != b.Int64 || a.Uint64 != b.Uint64 ||
		a.Float64 != b.Float64 || a.Complex128 != b.Complex128 || a.Text != b.Text {
		_, _ = fmt.Fprintf(sb, "%s- NumberNode{Text:%q IsInt:%v IsUint:%v IsFloat:%v IsComplex:%v Int64:%d Uint64:%d Float64:%g Complex128:%v}\n",
			indent, a.Text, a.IsInt, a.IsUint, a.IsFloat, a.IsComplex, a.Int64, a.Uint64, a.Float64, a.Complex128)
		_, _ = fmt.Fprintf(sb, "%s+ NumberNode{Text:%q IsInt:%v IsUint:%v IsFloat:%v IsComplex:%v Int64:%d Uint64:%d Float64:%g Complex128:%v}\n",
			indent, b.Text, b.IsInt, b.IsUint, b.IsFloat, b.IsComplex, b.Int64, b.Uint64, b.Float64, b.Complex128)
	}
}

func diffBranch(sb *strings.Builder, a, b *BranchNode, indent string) {
	if a.StartTrimLeft != b.StartTrimLeft || a.StartTrimRight != b.StartTrimRight {
		_, _ = fmt.Fprintf(sb, "%s  BranchNode.StartTrim: (%v,%v) vs (%v,%v)\n", indent, a.StartTrimLeft, a.StartTrimRight, b.StartTrimLeft, b.StartTrimRight)
	}
	if a.ElseTrimLeft != b.ElseTrimLeft || a.ElseTrimRight != b.ElseTrimRight {
		_, _ = fmt.Fprintf(sb, "%s  BranchNode.ElseTrim: (%v,%v) vs (%v,%v)\n", indent, a.ElseTrimLeft, a.ElseTrimRight, b.ElseTrimLeft, b.ElseTrimRight)
	}
	if a.EndTrimLeft != b.EndTrimLeft || a.EndTrimRight != b.EndTrimRight {
		_, _ = fmt.Fprintf(sb, "%s  BranchNode.EndTrim: (%v,%v) vs (%v,%v)\n", indent, a.EndTrimLeft, a.EndTrimRight, b.EndTrimLeft, b.EndTrimRight)
	}
	diffNodesAt(sb, a.Pipe, b.Pipe, indent+"  ")
	diffNodesAt(sb, a.List, b.List, indent+"  ")
	diffNodesAt(sb, a.ElseList, b.ElseList, indent+"  ")
}

func diffNodeSlices(sb *strings.Builder, a, b []Node, indent string) {
	if len(a) != len(b) {
		_, _ = fmt.Fprintf(sb, "%slen: %d vs %d\n", indent, len(a), len(b))
	}
	for i := range min(len(a), len(b)) {
		diffNodesAt(sb, a[i], b[i], indent)
	}
	for i := len(b); i < len(a); i++ {
		printNode(sb, a[i], indent, "-")
	}
	for i := len(a); i < len(b); i++ {
		printNode(sb, b[i], indent, "+")
	}
}

func printNode(sb *strings.Builder, n Node, indent, prefix string) {
	if isNodeNil(n) {
		_, _ = fmt.Fprintf(sb, "%s%s <nil>\n", indent, prefix)
		return
	}
	switch n := n.(type) {
	case *ListNode:
		_, _ = fmt.Fprintf(sb, "%s%s ListNode (len %d)\n", indent, prefix, len(n.Nodes))
	case *TextNode:
		_, _ = fmt.Fprintf(sb, "%s%s TextNode %q\n", indent, prefix, n.Text)
	case *CommentNode:
		_, _ = fmt.Fprintf(sb, "%s%s CommentNode %q\n", indent, prefix, n.Text)
	case *PipeNode:
		_, _ = fmt.Fprintf(sb, "%s%s PipeNode (Decl:%d Commands:%d IsAssign:%v)\n", indent, prefix, len(n.Decl), len(n.Commands), n.IsAssign)
	case *ActionNode:
		_, _ = fmt.Fprintf(sb, "%s%s ActionNode\n", indent, prefix)
	case *CommandNode:
		_, _ = fmt.Fprintf(sb, "%s%s CommandNode (Args:%d)\n", indent, prefix, len(n.Args))
	case *IdentifierNode:
		_, _ = fmt.Fprintf(sb, "%s%s IdentifierNode %q\n", indent, prefix, n.Ident)
	case *VariableNode:
		_, _ = fmt.Fprintf(sb, "%s%s VariableNode %v\n", indent, prefix, n.Ident)
	case *DotNode:
		_, _ = fmt.Fprintf(sb, "%s%s DotNode\n", indent, prefix)
	case *NilNode:
		_, _ = fmt.Fprintf(sb, "%s%s NilNode\n", indent, prefix)
	case *FieldNode:
		_, _ = fmt.Fprintf(sb, "%s%s FieldNode %v\n", indent, prefix, n.Ident)
	case *ChainNode:
		_, _ = fmt.Fprintf(sb, "%s%s ChainNode (Field:%v)\n", indent, prefix, n.Field)
	case *BoolNode:
		_, _ = fmt.Fprintf(sb, "%s%s BoolNode %v\n", indent, prefix, n.True)
	case *NumberNode:
		_, _ = fmt.Fprintf(sb, "%s%s NumberNode %q\n", indent, prefix, n.Text)
	case *StringNode:
		_, _ = fmt.Fprintf(sb, "%s%s StringNode %q\n", indent, prefix, n.Quoted)
	case *IfNode:
		_, _ = fmt.Fprintf(sb, "%s%s IfNode\n", indent, prefix)
	case *RangeNode:
		_, _ = fmt.Fprintf(sb, "%s%s RangeNode\n", indent, prefix)
	case *WithNode:
		_, _ = fmt.Fprintf(sb, "%s%s WithNode\n", indent, prefix)
	case *BreakNode:
		_, _ = fmt.Fprintf(sb, "%s%s BreakNode\n", indent, prefix)
	case *ContinueNode:
		_, _ = fmt.Fprintf(sb, "%s%s ContinueNode\n", indent, prefix)
	case *TemplateNode:
		_, _ = fmt.Fprintf(sb, "%s%s TemplateNode %q\n", indent, prefix, n.Name)
	case *HTMLDocumentFragmentNode:
		_, _ = fmt.Fprintf(sb, "%s%s HTMLDocumentFragmentNode (len %d)\n", indent, prefix, len(n.Children))
	case *HTMLElementNode:
		_, _ = fmt.Fprintf(sb, "%s%s HTMLElementNode <%s>\n", indent, prefix, n.TagName)
	case *HTMLAttributeNode:
		_, _ = fmt.Fprintf(sb, "%s%s HTMLAttributeNode\n", indent, prefix)
	case *HTMLCommentNode:
		_, _ = fmt.Fprintf(sb, "%s%s HTMLCommentNode\n", indent, prefix)
	case *HTMLDoctypeNode:
		_, _ = fmt.Fprintf(sb, "%s%s HTMLDoctypeNode %q\n", indent, prefix, n.Text)
	case *DefineNode:
		_, _ = fmt.Fprintf(sb, "%s%s DefineNode %q\n", indent, prefix, n.Name)
	case *BlockNode:
		_, _ = fmt.Fprintf(sb, "%s%s BlockNode %q\n", indent, prefix, n.Name)
	default:
		_, _ = fmt.Fprintf(sb, "%s%s %T\n", indent, prefix, n)
	}
}
