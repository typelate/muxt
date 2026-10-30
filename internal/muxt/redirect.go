package muxt

import (
	"slices"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/html"
)

// analyzeRedirectCalls sets canRedirect on each definition.
func analyzeRedirectCalls(ts *html.Template, defs []Definition) {
	for i := range defs {
		t := ts.Lookup(defs[i].name)
		if t == nil || t.Tree == nil {
			continue
		}
		defs[i].canRedirect = canTemplateRedirect(t.Tree.Root, ts, make(map[string]bool))
	}
}

// walkTemplateCommands visits every command in a template's tree, following
// {{template}} invocations, and stops at the first command visit accepts.
//
// One traversal serves everything that asks "does this template call X":
// deciding whether to emit the redirect block, and rejecting a call that
// cannot work. Two walks would have to agree about which nodes carry
// commands, and nothing would notice when they stopped.
//
// dotIsTemplateData says whether the dot in force where a command sits is
// still the TemplateData the handler passed in. It is passed to visit
// rather than acted on here, because the two callers want different
// things from it: the redirect analysis is deliberately conservative and
// ignores it, while a diagnostic must not claim a method belongs to
// TemplateData when the dot has been rebound.
//
// visited stops a template that reaches itself.
func walkTemplateCommands(node template.Node, ts *html.Template, visited map[string]bool, dotIsTemplateData bool, visit func(*template.CommandNode, bool) bool) bool {
	w := commandWalker{ts: ts, visited: visited, visit: visit}
	return w.node(node, dotIsTemplateData)
}

type commandWalker struct {
	ts      *html.Template
	visited map[string]bool
	visit   func(*template.CommandNode, bool) bool
}

func (w *commandWalker) node(node template.Node, dot bool) bool {
	switch n := node.(type) {
	case *template.ListNode:
		return n != nil && slices.ContainsFunc(n.Nodes, func(child template.Node) bool { return w.node(child, dot) })
	case *template.ActionNode:
		return w.pipe(n.Pipe, dot)
	case *template.PipeNode:
		return w.pipe(n, dot)
	case *template.IfNode:
		// An if does not rebind dot, in either branch.
		return w.branch(&n.BranchNode, dot, dot)
	case *template.WithNode:
		// Inside the body dot is whatever the with selected; the else
		// branch runs with the dot the with was written under.
		return w.branch(&n.BranchNode, false, dot)
	case *template.RangeNode:
		// Inside the body dot is one element of what was ranged over.
		return w.branch(&n.BranchNode, false, dot)
	case *template.TemplateNode:
		return w.template(n, dot)
	}
	return false
}

func (w *commandWalker) pipe(pipe *template.PipeNode, dot bool) bool {
	if pipe == nil {
		return false
	}
	for _, cmd := range pipe.Commands {
		if w.visit(cmd, dot) || w.arguments(cmd, dot) {
			return true
		}
	}
	return false
}

// arguments walks the parenthesised pipelines among cmd's arguments: an
// argument is not a command of the pipeline it is written in. Chaining a field
// onto one -- (.Redirect "/x").Header -- leaves the pipeline inside the
// chain, where it still has to be walked.
func (w *commandWalker) arguments(cmd *template.CommandNode, dot bool) bool {
	for _, arg := range cmd.Args {
		if chain, ok := arg.(*template.ChainNode); ok {
			arg = chain.Node
		}
		if pipe, ok := arg.(*template.PipeNode); ok && w.pipe(pipe, dot) {
			return true
		}
	}
	return false
}

func (w *commandWalker) branch(branch *template.BranchNode, bodyDot, elseDot bool) bool {
	// The pipeline itself is evaluated with the dot in force outside the
	// branch, which is the one the else branch runs under too.
	return w.pipe(branch.Pipe, elseDot) ||
		w.node(branch.List, bodyDot) ||
		w.node(branch.ElseList, elseDot)
}

func (w *commandWalker) template(n *template.TemplateNode, dot bool) bool {
	// The argument is evaluated where the invocation is written.
	if w.pipe(n.Pipe, dot) {
		return true
	}
	if w.visited[n.Name] {
		return false
	}
	w.visited[n.Name] = true
	defer delete(w.visited, n.Name)

	called := w.ts.Lookup(n.Name)
	if called == nil || called.Tree == nil {
		return false
	}
	return w.node(called.Tree.Root, passesDotAlong(n, dot))
}

// passesDotAlong reports whether the template a {{template}} node invokes
// runs with the same dot the invocation was written under.
//
// Only {{template "x" .}} does. Written without an argument the called
// template runs with no dot at all, and any other argument is a value
// selected out of the current one.
func passesDotAlong(n *template.TemplateNode, dotIsTemplateData bool) bool {
	if !dotIsTemplateData || n.Pipe == nil || len(n.Pipe.Commands) != 1 {
		return false
	}
	cmd := n.Pipe.Commands[0]
	if len(cmd.Args) != 1 {
		return false
	}
	_, isDot := cmd.Args[0].(*template.DotNode)
	return isDot
}

// canTemplateRedirect reports whether a template, or one it calls, can reach
// the Redirect method.
//
// It is deliberately conservative: passing TemplateData to a function, or
// calling a method that is not known to be safe, counts, and it does not
// care whether dot has been rebound. Emitting the redirect block for a
// template that never redirects costs nothing, and omitting it for one
// that does is the bug this guards.
func canTemplateRedirect(node template.Node, ts *html.Template, visited map[string]bool) bool {
	return walkTemplateCommands(node, ts, visited, true, func(cmd *template.CommandNode, _ bool) bool {
		return containsRedirectCall(cmd) || callsMethodOnTemplateData(cmd)
	})
}

// isRedirectMethod returns true if the method name is a redirect method
// that sets the redirectURL field on TemplateData
func isRedirectMethod(methodName string) bool {
	switch methodName {
	case "Redirect", "RedirectMultipleChoices", "RedirectMovedPermanently", "RedirectFound", "RedirectSeeOther":
		return true
	}
	return false
}

// containsRedirectCall reports whether a command names a redirect method in a
// field, chain or variable ($.Redirect, $d.Redirect), wherever dot or the
// variable points. A parenthesised pipeline in a chain is its own command to
// the walker, so it is not looked into here.
func containsRedirectCall(cmd *template.CommandNode) bool {
	if cmd == nil {
		return false
	}
	return slices.ContainsFunc(cmd.Args, func(arg template.Node) bool {
		switch a := arg.(type) {
		case *template.FieldNode:
			return slices.ContainsFunc(a.Ident, isRedirectMethod)
		case *template.ChainNode:
			return slices.ContainsFunc(a.Field, isRedirectMethod)
		case *template.VariableNode:
			return len(a.Ident) > 1 && slices.ContainsFunc(a.Ident[1:], isRedirectMethod)
		}
		return false
	})
}

// callsMethodOnTemplateData reports whether a command may call a TemplateData
// method that is not known to be safe: by naming one on dot or on a variable,
// which may hold dot ({{$d := .}}), or by handing a function dot, a variable
// or a chain, which it might call methods on.
func callsMethodOnTemplateData(cmd *template.CommandNode) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	if _, isFunctionCall := cmd.Args[0].(*template.IdentifierNode); isFunctionCall && slices.ContainsFunc(cmd.Args[1:], mayHoldTemplateData) {
		return true
	}
	return slices.ContainsFunc(cmd.Args, hasUnsafeField)
}

func mayHoldTemplateData(arg template.Node) bool {
	switch arg.(type) {
	case *template.DotNode, *template.ChainNode, *template.VariableNode:
		return true
	}
	return false
}

func hasUnsafeField(arg template.Node) bool {
	switch a := arg.(type) {
	case *template.FieldNode:
		return !isSafeTemplateDataMethod(a.Ident[0])
	case *template.VariableNode:
		return len(a.Ident) > 1 && !isSafeTemplateDataMethod(a.Ident[1])
	}
	return false
}

// isSafeTemplateDataMethod reports whether a TemplateData method definitely
// does not set redirectURL. Fields after the first are on the returned value,
// not on TemplateData, so only the first identifier of a chain matters.
func isSafeTemplateDataMethod(methodName string) bool {
	switch methodName {
	case "Path", "Result", "Request", "Receiver", "Ok", "Err", "MuxtVersion", "StatusCode", "Header":
		return true
	}
	return false
}
