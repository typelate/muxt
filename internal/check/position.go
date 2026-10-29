package check

import (
	"cmp"
	"go/token"

	"github.com/typelate/muxt/internal/template"
)

// ParseNodePosition returns the source position of node within tree as a
// go/token.Position: 1-based line/column (from Tree.Position), the node's
// byte offset (from Node.Start), and the tree's filename when known,
// falling back to the parse name. It is the free-function analogue of
// Tree.Position for callers — the ported muxt analysis code — that hold
// only a (tree, node) pair.
func ParseNodePosition(tree *template.Tree, node template.Node) token.Position {
	p := tree.Position(node)
	return token.Position{
		Filename: cmp.Or(tree.Filepath, p.ParseName),
		Line:     p.Line,
		Column:   p.Column,
		Offset:   int(node.Start()),
	}
}
