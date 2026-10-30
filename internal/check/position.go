package check

import (
	"cmp"
	"go/token"

	"github.com/typelate/muxt/internal/template"
)

// ParseNodePosition returns the source position of node within tree as a
// go/token.Position: 1-based line/column (from Tree.Position), the node's
// byte offset (from Node.Start, but see below), and the tree's filename when known,
// falling back to the parse name. It is the free-function analogue of
// Tree.Position for callers — the ported muxt analysis code — that hold
// only a (tree, node) pair.
//
// A {{template}} call, and the call a {{block}} stands for, is located at
// its quoted name, where text/template's parser placed it, rather than at
// its left delimiter. The tree's root is located where text/template's
// parser started it, past the whitespace a {{define ... -}} trims (see
// Tree.RootStart).
func ParseNodePosition(tree *template.Tree, node template.Node) token.Position {
	p := tree.Position(node)
	offset := node.Start()
	if name, ok := templateNameStart(node); ok {
		at := tree.PositionAt(name)
		p.Line, p.Column, offset = at.Line, at.Column, name
	} else if root, ok := node.(*template.ListNode); ok && root == tree.Root {
		offset = tree.RootStart()
		at := tree.PositionAt(offset)
		p.Line, p.Column = at.Line, at.Column
	}
	return token.Position{
		Filename: cmp.Or(tree.Filepath, p.ParseName),
		Line:     p.Line,
		Column:   p.Column,
		Offset:   int(offset),
	}
}
