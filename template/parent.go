package template

import "reflect"

// nodeParent is embedded in the structural nodes of the tree to record each
// node's lexical parent. Expression-level nodes (numbers, identifiers,
// pipelines, and other action internals) do not participate in tree navigation
// and intentionally do not carry a parent.
type nodeParent struct {
	parent Node
}

// Parent returns the node's lexical parent, or nil for a root.
//
// A DefineNode and BlockNode are detached roots: their Parent is nil even
// though they are written inside another template's source. The logical
// parents of a define or block body are the {{template}} and {{block}} call
// sites that invoke it, which are resolved by callers rather than recorded
// here.
func (n nodeParent) Parent() Node { return n.parent }

func (n *nodeParent) setParent(p Node) { n.parent = p }

// parentSetter is implemented by every structural node via the embedded
// nodeParent.
type parentSetter interface {
	setParent(Node)
}

// setParents walks the tree and assigns every structural node its lexical
// parent. It is called once after parsing.
func (t *Tree) setParents() {
	assignParents(t.Root, nil)
}

func assignParents(n, parent Node) {
	if isNilNode(n) {
		return
	}
	self := parent
	switch n.(type) {
	case *DefineNode, *BlockNode:
		// Detached root: do not parent a define/block to its enclosing document.
		self = nil
	}
	if ps, ok := n.(parentSetter); ok {
		ps.setParent(self)
	}
	forEachStructuralChild(n, func(child Node) {
		assignParents(child, n)
	})
}

// isNilNode reports whether n is a nil interface or a typed nil pointer.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// forEachStructuralChild calls fn for each child node that participates in tree
// navigation, in source order.
func forEachStructuralChild(n Node, fn func(Node)) {
	switch n := n.(type) {
	case *ListNode:
		for _, c := range n.Nodes {
			fn(c)
		}
	case *HTMLDocumentFragmentNode:
		for _, c := range n.Children {
			fn(c)
		}
	case *HTMLElementNode:
		for _, a := range n.Attr {
			fn(a)
		}
		for _, c := range n.Children {
			fn(c)
		}
	case *HTMLAttributeNode:
		for _, k := range n.Key {
			fn(k)
		}
		for _, v := range n.Value {
			fn(v)
		}
	case *HTMLCommentNode:
		for _, c := range n.Children {
			fn(c)
		}
	case *IfNode:
		branchChildren(&n.BranchNode, fn)
	case *RangeNode:
		branchChildren(&n.BranchNode, fn)
	case *WithNode:
		branchChildren(&n.BranchNode, fn)
	case *DefineNode:
		fn(n.Body)
	case *BlockNode:
		fn(n.Body)
	}
}

func branchChildren(b *BranchNode, fn func(Node)) {
	if b.List != nil {
		fn(b.List)
	}
	if b.ElseList != nil {
		fn(b.ElseList)
	}
}
