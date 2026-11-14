package mutation

import (
	"go/types"

	"github.com/typelate/htt"

	"github.com/typelate/muxt/internal/check"
)

// action is one action of a template, with what the mutations applied to
// it need to know.
type action struct {
	// node is the action itself. Its kind decides which variations
	// apply, so the walker reports it rather than interpreting it.
	node htt.Node

	// pipe is the pipeline the action evaluates, nil for a {{template}}
	// invocation written without an argument.
	pipe *htt.PipeNode

	// dot is the type in force where the action sits, already narrowed
	// by any range or with it is written inside.
	dot types.Type

	// region is where the action is written, and index is its position
	// among the source's regions, which is how a construct finds the
	// else and end that belong to it.
	region region
	index  int
}

// walkActions reports every action of a template, in the order they are
// mutated, with the type of dot in force at each.
//
// The order is the walk's own: an action before the bodies it encloses,
// and a body before the else beside it.
func walkActions(found []region, dot types.Type, functions check.Functions, root htt.Node, visit func(action)) {
	var walk func(htt.Node, types.Type)
	var walkNodes func([]htt.Node, types.Type)
	report := func(node htt.Node, pipe *htt.PipeNode, at int, dot types.Type) {
		index, r, ok := regionAt(found, at)
		if !ok {
			return
		}
		visit(action{
			node:   node,
			pipe:   pipe,
			dot:    dot,
			region: r,
			index:  index,
		})
	}

	walk = func(node htt.Node, dot types.Type) {
		switch n := node.(type) {
		case *htt.ListNode:
			if n == nil {
				return
			}
			for _, child := range n.Nodes {
				walk(child, dot)
			}
		case *htt.ActionNode:
			report(n, n.Pipe, int(n.Pipe.Start()), dot)
		case *htt.IfNode:
			report(n, n.Pipe, int(n.Pipe.Start()), dot)
			walk(n.List, dot)
			walk(n.ElseList, dot)
		case *htt.WithNode:
			report(n, n.Pipe, int(n.Pipe.Start()), dot)
			// Inside the body, dot is what the with selected.
			walk(n.List, withDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *htt.RangeNode:
			report(n, n.Pipe, int(n.Pipe.Start()), dot)
			// Inside the body, dot is one element of what was ranged over.
			walk(n.List, rangeDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *htt.TemplateNode:
			// A {{template}} node's pipeline may be absent entirely, so
			// the node's own start locates it.
			report(n, n.Pipe, int(n.Start()), dot)
		case *htt.BlockNode:
			// text/template turns a block into a {{template}} call of the
			// tree it defines, and that tree is mutated on its own; the
			// copied parser keeps the clause, and its body, inline.
			report(n, n.Pipe, int(n.Start()), dot)
		case *htt.HTMLDocumentFragmentNode:
			walkNodes(n.Children, dot)
		case *htt.HTMLElementNode:
			walkNodes(n.Attr, dot)
			walkNodes(n.Children, dot)
		case *htt.HTMLAttributeNode:
			walkNodes(n.Key, dot)
			walkNodes(n.Value, dot)
		case *htt.HTMLCommentNode:
			walkNodes(n.Children, dot)
		}
	}
	// The copied parser builds HTML nodes where text/template kept flat
	// text; their children are walked in source order.
	walkNodes = func(nodes []htt.Node, dot types.Type) {
		for _, child := range nodes {
			walk(child, dot)
		}
	}
	walk(root, dot)
}
