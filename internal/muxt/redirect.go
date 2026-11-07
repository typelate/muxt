package muxt

import (
	"slices"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/html"
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
// still the TemplateData the handler passed in. A range rebinds it. A with
// and a {{template}} call bind it to what their pipeline selects, and
// keepsDot says whether that is the TemplateData: the redirect analysis
// answers yes whenever it may be, so that no redirect is missed, while a
// diagnostic answers yes only when it provably is, so that it never claims
// a method belongs to TemplateData when the dot has been rebound.
//
// visited stops a template that reaches itself.
func walkTemplateCommands(node template.Node, ts *html.Template, visited map[string]bool, dotIsTemplateData bool, keepsDot func(*template.PipeNode, bool) bool, visit func(*template.CommandNode, bool) bool) bool {
	w := commandWalker{ts: ts, visited: visited, keepsDot: keepsDot, visit: visit}
	return w.node(node, dotIsTemplateData)
}

type commandWalker struct {
	ts       *html.Template
	visited  map[string]bool
	keepsDot func(*template.PipeNode, bool) bool
	visit    func(*template.CommandNode, bool) bool
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
		return w.branch(&n.BranchNode, w.keepsDot(n.Pipe, dot), dot)
	case *template.RangeNode:
		// Inside the body dot is one element of what was ranged over.
		return w.branch(&n.BranchNode, false, dot)
	case *template.TemplateNode:
		return w.template(n.Name, n.Pipe, dot)
	case *template.BlockNode:
		// text/template turns a block into a {{template}} call of the
		// tree it defines; the copied parser keeps the clause inline.
		return w.template(n.Name, n.Pipe, dot)
	case *template.HTMLDocumentFragmentNode:
		return w.nodes(n.Children, dot)
	case *template.HTMLElementNode:
		return w.nodes(n.Attr, dot) || w.nodes(n.Children, dot)
	case *template.HTMLAttributeNode:
		return w.nodes(n.Key, dot) || w.nodes(n.Value, dot)
	case *template.HTMLCommentNode:
		return w.nodes(n.Children, dot)
	}
	return false
}

// nodes walks the children of an HTML node, which the copied parser
// builds where text/template kept flat text, in source order.
func (w *commandWalker) nodes(children []template.Node, dot bool) bool {
	return slices.ContainsFunc(children, func(child template.Node) bool { return w.node(child, dot) })
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

func (w *commandWalker) template(name string, pipe *template.PipeNode, dot bool) bool {
	// The argument is evaluated where the invocation is written.
	if w.pipe(pipe, dot) {
		return true
	}
	if w.visited[name] {
		return false
	}
	w.visited[name] = true
	defer delete(w.visited, name)

	called := w.ts.Lookup(name)
	if called == nil || called.Tree == nil {
		return false
	}
	// Written without an argument the called template runs with no dot
	// at all.
	return w.node(called.Tree.Root, pipe != nil && w.keepsDot(pipe, dot))
}

// mayBeTemplateData reports whether the value pipe selects may be the
// TemplateData the handler passed in: dot while dot still is, the root $,
// any other variable, which may hold dot, a method of TemplateData that
// returns it or that muxt does not know, a chain, a function call, or a
// pipeline of more than one command. A field that is known to select
// something else, such as .Result, and a literal are not.
func mayBeTemplateData(pipe *template.PipeNode, dotIsTemplateData bool) bool {
	if pipe == nil {
		return false
	}
	cmds := pipe.HeadCommands()
	if len(cmds) != 1 || len(cmds[0].Args) == 0 {
		return len(cmds) > 1
	}
	switch arg := cmds[0].Args[0].(type) {
	case *template.DotNode:
		return dotIsTemplateData
	case *template.FieldNode:
		return dotIsTemplateData && !selectsOutOfTemplateData(arg.Ident[0])
	case *template.VariableNode:
		return len(arg.Ident) == 1 || !selectsOutOfTemplateData(arg.Ident[1]) || arg.Ident[0] != "$"
	case *template.PipeNode:
		return mayBeTemplateData(arg, dotIsTemplateData)
	case *template.StringNode, *template.NumberNode, *template.BoolNode, *template.NilNode:
		return false
	}
	return true
}

// selectsOutOfTemplateData reports whether a TemplateData method returns
// something other than the TemplateData itself.
func selectsOutOfTemplateData(methodName string) bool {
	return isSafeTemplateDataMethod(methodName) && !returnsTemplateData(methodName)
}

// canTemplateRedirect reports whether a template, or one it calls, can reach
// the Redirect method.
//
// A field on dot counts only where dot is still TemplateData: inside a
// with or a range, .Redirect names a field of whatever was selected. The
// root, $, is always TemplateData. It is otherwise conservative: passing
// TemplateData to a function, holding it in a variable, or calling a
// method that is not known to be safe, counts. Emitting the redirect block
// for a template that never redirects costs nothing, and omitting it for
// one that does is the bug this guards.
func canTemplateRedirect(node template.Node, ts *html.Template, visited map[string]bool) bool {
	return walkTemplateCommands(node, ts, visited, true, mayBeTemplateData, func(cmd *template.CommandNode, dot bool) bool {
		return containsRedirectCall(cmd, dot) || callsMethodOnTemplateData(cmd, dot)
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
// field of dot, while dot is TemplateData, or in a chain or variable
// ($.Redirect, $d.Redirect), wherever the variable points. A parenthesised
// pipeline in a chain is its own command to the walker, so it is not looked
// into here.
func containsRedirectCall(cmd *template.CommandNode, dot bool) bool {
	if cmd == nil {
		return false
	}
	return slices.ContainsFunc(cmd.Args, func(arg template.Node) bool {
		switch a := arg.(type) {
		case *template.FieldNode:
			return dot && slices.ContainsFunc(a.Ident, isRedirectMethod)
		case *template.ChainNode:
			return slices.ContainsFunc(a.Field, isRedirectMethod)
		case *template.VariableNode:
			return len(a.Ident) > 1 && slices.ContainsFunc(a.Ident[1:], isRedirectMethod)
		}
		return false
	})
}

// callsMethodOnTemplateData reports whether a command may call a TemplateData
// method that is not known to be safe: by naming one on dot, while dot is
// TemplateData, or on a variable, which may hold dot ({{$d := .}}), or by
// handing a function dot, a variable or a chain, which it might call
// methods on.
func callsMethodOnTemplateData(cmd *template.CommandNode, dot bool) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	if _, isFunctionCall := cmd.Args[0].(*template.IdentifierNode); isFunctionCall && slices.ContainsFunc(cmd.Args[1:], func(arg template.Node) bool { return mayHoldTemplateData(arg, dot) }) {
		return true
	}
	return slices.ContainsFunc(cmd.Args, func(arg template.Node) bool { return hasUnsafeField(arg, dot) })
}

func mayHoldTemplateData(arg template.Node, dot bool) bool {
	switch arg.(type) {
	case *template.DotNode:
		return dot
	case *template.ChainNode, *template.VariableNode:
		return true
	}
	return false
}

func hasUnsafeField(arg template.Node, dot bool) bool {
	switch a := arg.(type) {
	case *template.FieldNode:
		return dot && !isSafeTemplateDataMethod(a.Ident[0])
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
