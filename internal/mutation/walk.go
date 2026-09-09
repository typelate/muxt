package mutation

import (
	"go/types"
	"text/template/parse"

	"github.com/typelate/check"
)

// action is one action of a template, as everything downstream sees it.
//
// Both the mutants and the identifiers are built from this: the same
// actions, in the same order, with the same type of dot. Deriving them
// from one walk is what keeps them describing the same thing -- two
// walks would have to agree on traversal order and on how dot narrows,
// and nothing would notice when they stopped.
type action struct {
	// index is the action's place in the walk, counting from one. It is
	// what tells two identically written actions apart.
	index int

	// node is the action itself. Its kind decides which variations
	// apply, so the walker reports it rather than interpreting it.
	node parse.Node

	// pipe is the pipeline the action evaluates, nil for a {{template}}
	// invocation written without an argument.
	pipe *parse.PipeNode

	// dot is the type in force where the action sits, already narrowed
	// by any range or with it is written inside.
	dot types.Type

	// region is where the action is written.
	region region

	// text is the action as written, delimiters included.
	text string
}

// operands returns the leaf inputs the action reads.
func (a action) operands(text string) []operand {
	return operands(text, a.dot, a.pipe)
}

// walkActions reports every action of a template, in the order they are
// mutated, with the type of dot in force at each.
//
// The order is the walk's own: an action before the bodies it encloses,
// and a body before the else beside it. Everything that consumes actions
// depends on that order, since an action's place in it is part of its
// identity.
func walkActions(text string, found []region, dot types.Type, functions check.Functions, root parse.Node, visit func(action)) {
	seq := 0

	var walk func(parse.Node, types.Type)
	report := func(node parse.Node, pipe *parse.PipeNode, at int, dot types.Type) {
		_, r, ok := regionAt(found, at)
		if !ok {
			return
		}
		seq++
		visit(action{
			index:  seq,
			node:   node,
			pipe:   pipe,
			dot:    dot,
			region: r,
			text:   text[r.start:r.end],
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

// invocation is a {{template}} call an action makes, with the type of dot
// it passes on.
func (a action) invocation(text string, functions check.Functions) (templateCall, bool) {
	node, ok := a.node.(*parse.TemplateNode)
	if !ok {
		return templateCall{}, false
	}
	// A call written without an argument renders with no dot at all,
	// which is a different execution from one passing a value along.
	var passed types.Type
	if a.pipe != nil {
		if resolved, ok := pipelineType(a.dot, a.pipe, functions); ok {
			passed = resolved
		}
	}
	return templateCall{name: node.Name, dot: passed}, true
}
