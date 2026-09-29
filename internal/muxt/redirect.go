package muxt

import (
	"html/template"
	"slices"
	"text/template/parse"
)

// analyzeRedirectCalls performs static analysis on all templates to determine
// which ones can call the Redirect method. It updates the canRedirect field
// on each Definition in the templates slice.
func analyzeRedirectCalls(ts *template.Template, defs []Definition) {
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
func walkTemplateCommands(node parse.Node, ts *template.Template, visited map[string]bool, dotIsTemplateData bool, visit func(*parse.CommandNode, bool) bool) bool {
	w := commandWalker{ts: ts, visited: visited, visit: visit}
	return w.node(node, dotIsTemplateData)
}

type commandWalker struct {
	ts      *template.Template
	visited map[string]bool
	visit   func(*parse.CommandNode, bool) bool
}

func (w *commandWalker) node(node parse.Node, dot bool) bool {
	switch n := node.(type) {
	case *parse.ListNode:
		return n != nil && slices.ContainsFunc(n.Nodes, func(child parse.Node) bool { return w.node(child, dot) })
	case *parse.ActionNode:
		return w.pipe(n.Pipe, dot)
	case *parse.PipeNode:
		return w.pipe(n, dot)
	case *parse.IfNode:
		// An if does not rebind dot, in either branch.
		return w.branch(&n.BranchNode, dot, dot)
	case *parse.WithNode:
		// Inside the body dot is whatever the with selected; the else
		// branch runs with the dot the with was written under.
		return w.branch(&n.BranchNode, false, dot)
	case *parse.RangeNode:
		// Inside the body dot is one element of what was ranged over.
		return w.branch(&n.BranchNode, false, dot)
	case *parse.TemplateNode:
		return w.template(n, dot)
	}
	return false
}

func (w *commandWalker) pipe(pipe *parse.PipeNode, dot bool) bool {
	if pipe == nil {
		return false
	}
	for _, cmd := range pipe.Cmds {
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
func (w *commandWalker) arguments(cmd *parse.CommandNode, dot bool) bool {
	for _, arg := range cmd.Args {
		if chain, ok := arg.(*parse.ChainNode); ok {
			arg = chain.Node
		}
		if pipe, ok := arg.(*parse.PipeNode); ok && w.pipe(pipe, dot) {
			return true
		}
	}
	return false
}

func (w *commandWalker) branch(branch *parse.BranchNode, bodyDot, elseDot bool) bool {
	// The pipeline itself is evaluated with the dot in force outside the
	// branch, which is the one the else branch runs under too.
	return w.pipe(branch.Pipe, elseDot) ||
		w.node(branch.List, bodyDot) ||
		w.node(branch.ElseList, elseDot)
}

func (w *commandWalker) template(n *parse.TemplateNode, dot bool) bool {
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
func passesDotAlong(n *parse.TemplateNode, dotIsTemplateData bool) bool {
	if !dotIsTemplateData || n.Pipe == nil || len(n.Pipe.Cmds) != 1 {
		return false
	}
	cmd := n.Pipe.Cmds[0]
	if len(cmd.Args) != 1 {
		return false
	}
	_, isDot := cmd.Args[0].(*parse.DotNode)
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
func canTemplateRedirect(node parse.Node, ts *template.Template, visited map[string]bool) bool {
	return walkTemplateCommands(node, ts, visited, true, func(cmd *parse.CommandNode, _ bool) bool {
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

// containsRedirectCall checks if a command node contains a call to a redirect method
func containsRedirectCall(cmd *parse.CommandNode) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}

	for _, arg := range cmd.Args {
		if field, ok := arg.(*parse.FieldNode); ok {
			if len(field.Ident) > 0 && isRedirectMethod(field.Ident[len(field.Ident)-1]) {
				return true
			}
			for _, ident := range field.Ident {
				if isRedirectMethod(ident) {
					return true
				}
			}
		}
		// Check for chain nodes like .field.Redirect or (.Redirect ...).Header
		if chain, ok := arg.(*parse.ChainNode); ok {
			for _, field := range chain.Field {
				if isRedirectMethod(field) {
					return true
				}
			}
			if chainNode, ok := chain.Node.(*parse.PipeNode); ok {
				for _, chainCmd := range chainNode.Cmds {
					if containsRedirectCall(chainCmd) {
						return true
					}
				}
			}
		}
	}
	return false
}

func callsMethodOnTemplateData(cmd *parse.CommandNode) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	firstArg := cmd.Args[0]
	if _, ok := firstArg.(*parse.IdentifierNode); ok {
		if len(cmd.Args) > 1 {
			// This is a function call with arguments
			for i := 1; i < len(cmd.Args); i++ {
				switch arg := cmd.Args[i].(type) {
				case *parse.DotNode:
					// Bare . is being passed - this is the full TemplateData
					// Be conservative: function might call methods on it
					return true
				case *parse.FieldNode:
					if !isAllSafeMethods(arg.Ident) {
						return true
					}
				case *parse.ChainNode:
					// A chain is being passed, be conservative
					return true
				}
			}
		}
	}

	for _, arg := range cmd.Args {
		if field, ok := arg.(*parse.FieldNode); ok {
			if !isAllSafeMethods(field.Ident) {
				return true
			}
		}
	}

	return false
}

// isAllSafeMethods checks if all identifiers in a field chain are safe methods
func isAllSafeMethods(idents []string) bool {
	if len(idents) == 0 {
		return true
	}
	// First identifier must be a safe TemplateData method
	if !isSafeTemplateDataMethod(idents[0]) {
		return false
	}
	// If there are more identifiers, we're chaining off the result
	// e.g. `.Request.Method` - this is safe if Request is safe
	// (subsequent fields/methods are on the returned type, not TemplateData)
	return true
}

// isSafeTemplateDataMethod returns true for TemplateData methods that definitely
// don't set redirectURL (i.e., don't call Redirect internally)
func isSafeTemplateDataMethod(methodName string) bool {
	safeMethodsSet := map[string]bool{
		"Path":        true, // returns TemplateRoutePaths
		"Result":      true, // returns T (the result type)
		"Request":     true, // returns *http.Request
		"Receiver":    true, // returns R (the receiver type)
		"Ok":          true, // returns bool
		"Err":         true, // returns error
		"MuxtVersion": true, // returns string
		"StatusCode":  true, // sets statusCode field, returns *TemplateData but doesn't set redirectURL
		"Header":      true, // sets response headers, returns *TemplateData but doesn't set redirectURL
	}
	return safeMethodsSet[methodName]
}
