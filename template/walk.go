package template

// Walk traverses the tree in source order, pre-order, calling fn for
// each node. It is a thin wrapper over WalkPath.
func Walk(tree *Tree, fn func(*Tree, Node)) {
	WalkPath(tree, func(t *Tree, _ []Node, n Node) { fn(t, n) })
}

// WalkPath traverses like Walk but also passes the stack of ancestor
// nodes, outermost first. Branch nodes (IfNode, RangeNode, WithNode)
// and list nodes appear in the path so callers can observe control
// flow between elements. fn must not retain path after returning; the
// backing array is reused.
func WalkPath(tree *Tree, fn func(t *Tree, path []Node, n Node)) {
	if tree == nil || tree.Root == nil {
		return
	}
	walkPathNode(tree, make([]Node, 0, 16), tree.Root, fn)
}

func walkPathNode(tree *Tree, path []Node, node Node, fn func(*Tree, []Node, Node)) {
	if node == nil {
		return
	}
	fn(tree, path, node)
	path = append(path, node)
	switch n := node.(type) {
	case *ListNode:
		for _, child := range n.Nodes {
			walkPathNode(tree, path, child, fn)
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			walkPathNode(tree, path, attr, fn)
		}
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	case *IfNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *RangeNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *WithNode:
		if n.List != nil {
			walkPathNode(tree, path, n.List, fn)
		}
		if n.ElseList != nil {
			walkPathNode(tree, path, n.ElseList, fn)
		}
	case *HTMLAttributeNode:
		for _, k := range n.Key {
			walkPathNode(tree, path, k, fn)
		}
		for _, v := range n.Value {
			walkPathNode(tree, path, v, fn)
		}
	case *DefineNode:
		walkPathNode(tree, path, n.Body, fn)
	case *BlockNode:
		walkPathNode(tree, path, n.Body, fn)
	case *HTMLCommentNode:
		for _, child := range n.Children {
			walkPathNode(tree, path, child, fn)
		}
	}
}
