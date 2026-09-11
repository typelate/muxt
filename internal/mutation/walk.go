package mutation

import (
	"go/types"
	"text/template/parse"

	"github.com/typelate/check"
)

// action is one action of a template, with what the mutations applied to
// it need to know.
type action struct {
	// node is the action itself. Its kind decides which variations
	// apply, so the walker reports it rather than interpreting it.
	node parse.Node

	// pipe is the pipeline the action evaluates, nil for a {{template}}
	// invocation written without an argument.
	pipe *parse.PipeNode

	// dot is the type in force where the action sits, already narrowed
	// by any range or with it is written inside.
	dot types.Type

	// region is where the action is written, and index is its position
	// among the source's regions, which is how a construct finds the
	// else and end that belong to it.
	region region
	index  int

	// text is the action as written, delimiters included.
	text string
}

// walkActions reports every action of a template, in the order they are
// mutated, with the type of dot in force at each.
//
// The order is the walk's own: an action before the bodies it encloses,
// and a body before the else beside it.
func walkActions(templateText string, found []region, dot types.Type, functions check.Functions, root parse.Node, visit func(action)) {
	var walk func(parse.Node, types.Type)
	report := func(node parse.Node, pipe *parse.PipeNode, at int, dot types.Type) {
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
			text:   templateText[r.start:r.end],
		})
	}

	walk = func(node parse.Node, dot types.Type) {
		switch n := node.(type) {
		case *parse.ListNode:
			if n == nil {
				return
			}
			for _, child := range n.Nodes {
				walk(child, dot)
			}
		case *parse.ActionNode:
			report(n, n.Pipe, int(n.Pipe.Position()), dot)
		case *parse.IfNode:
			report(n, n.Pipe, int(n.Pipe.Position()), dot)
			walk(n.List, dot)
			walk(n.ElseList, dot)
		case *parse.WithNode:
			report(n, n.Pipe, int(n.Pipe.Position()), dot)
			// Inside the body, dot is what the with selected.
			walk(n.List, withDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *parse.RangeNode:
			report(n, n.Pipe, int(n.Pipe.Position()), dot)
			// Inside the body, dot is one element of what was ranged over.
			walk(n.List, rangeDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *parse.TemplateNode:
			// A {{template}} node is positioned at its name, not at its
			// pipeline, which may be absent entirely.
			report(n, n.Pipe, int(n.Position()), dot)
		}
	}
	walk(root, dot)
}
