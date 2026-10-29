package check

import (
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// structFieldHasNonil reports whether the named field of t (a struct, or a
// pointer to one) carries the templatecheck:"nonil" tag, which promises the
// pointer field is always initialized and suppresses the W003 warning.
func structFieldHasNonil(t types.Type, field string) bool {
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := range st.NumFields() {
		if st.Field(i).Name() == field {
			return reflect.StructTag(st.Tag(i)).Get("templatecheck") == "nonil"
		}
	}
	return false
}

// Warning codes. They parallel the tooolbox/check fork so tooling and docs
// can share a vocabulary.
const (
	WarnNonStaticName        = "W001" // ExecuteTemplate name is not a string literal
	WarnUnusedTemplate       = "W002" // a parsed template is never referenced
	WarnUnguardedPointer     = "W003" // a field access on a pointer dot with no nil guard
	WarnInterfaceFieldAccess = "W004" // a field access on an interface dot
	WarnUnusedVariable       = "W005" // a declared {{$x := ...}} is never used
	WarnDeadBranch           = "W006" // a branch whose literal condition can never take it
	WarnInconsistentTemplate = "W007" // a {{template}} invoked with incompatible data types
)

// Warning is a non-fatal diagnostic: a likely bug that is not itself a type
// error. Unlike an Error it never fails the check; callers surface warnings
// separately (the LSP as Warning-severity diagnostics, the CLI behind -w).
//
// A warning is located either in a template — Tree and Node are set, and
// Tree.Position(Node) gives its span — or in Go source, when Pos is valid and
// Tree/Node are nil (e.g. a non-literal ExecuteTemplate name at its call
// site).
type Warning struct {
	Code    string
	Message string

	Tree *template.Tree
	Node template.Node

	Pos token.Pos
}

// emptyInterface is the type a value takes on after an unverifiable
// interface field access (W004), so the rest of the chain continues without
// cascading not-found errors.
var emptyInterface = types.NewInterfaceType(nil, nil).Complete()

// warn reports a template-located warning through Global.InspectWarning when
// one is set; otherwise it is a no-op, so checking without a warning sink
// costs nothing.
func (s *scope) warn(tree *template.Tree, node template.Node, code, format string, args ...any) {
	if s.global.InspectWarning == nil {
		return
	}
	s.global.InspectWarning(Warning{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Tree:    tree,
		Node:    node,
	})
}

// reportUnusedVariables walks tree and warns (W005) for every {{$x := ...}}
// declaration whose name is never referenced. It is conservative — a name
// used anywhere in the template counts as used, so shadowing never yields a
// false positive — and it ignores {{range}}/{{with}} loop variables, which
// are commonly partially used. It runs only when a warning sink is set.
func reportUnusedVariables(g *Global, tree *template.Tree) {
	if g.InspectWarning == nil || tree == nil || tree.Root == nil {
		return
	}
	var decls []*template.VariableNode
	used := make(map[string]bool)

	var visitPipe func(p *template.PipeNode, declares bool)
	visitPipe = func(p *template.PipeNode, declares bool) {
		if p == nil {
			return
		}
		for _, cmd := range p.Commands {
			for _, arg := range cmd.Args {
				switch a := arg.(type) {
				case *template.VariableNode:
					if len(a.Ident) > 0 {
						used[a.Ident[0]] = true
					}
				case *template.PipeNode:
					visitPipe(a, false)
				}
			}
		}
		if declares && !p.IsAssign {
			for _, d := range p.Decl {
				if len(d.Ident) > 0 {
					decls = append(decls, d)
				}
			}
		}
	}

	var visit func(n template.Node)
	visit = func(n template.Node) {
		switch v := n.(type) {
		case *template.ListNode:
			for _, c := range v.Nodes {
				visit(c)
			}
		case *template.ActionNode:
			visitPipe(v.Pipe, true)
		case *template.IfNode:
			visitPipe(v.Pipe, false)
			if v.List != nil {
				visit(v.List)
			}
			if v.ElseList != nil {
				visit(v.ElseList)
			}
		case *template.RangeNode:
			visitPipe(v.Pipe, false)
			if v.List != nil {
				visit(v.List)
			}
			if v.ElseList != nil {
				visit(v.ElseList)
			}
		case *template.WithNode:
			visitPipe(v.Pipe, false)
			if v.List != nil {
				visit(v.List)
			}
			if v.ElseList != nil {
				visit(v.ElseList)
			}
		case *template.TemplateNode:
			visitPipe(v.Pipe, false)
		case *template.HTMLDocumentFragmentNode:
			for _, c := range v.Children {
				visit(c)
			}
		case *template.HTMLElementNode:
			for _, a := range v.Attr {
				visit(a)
			}
			for _, c := range v.Children {
				visit(c)
			}
		case *template.HTMLAttributeNode:
			for _, k := range v.Key {
				visit(k)
			}
			for _, val := range v.Value {
				visit(val)
			}
		case *template.HTMLCommentNode:
			for _, c := range v.Children {
				visit(c)
			}
		}
	}
	visit(tree.Root)

	for _, d := range decls {
		if !used[d.Ident[0]] {
			g.InspectWarning(Warning{
				Code:    WarnUnusedVariable,
				Message: fmt.Sprintf("%s is declared but never used", d.Ident[0]),
				Tree:    tree,
				Node:    d,
			})
		}
	}
}

// guardedPath returns the dot-rooted field path a branch condition proves
// non-nil for its body, and whether the condition is such a guard. {{if .}}
// guards "" (the dot); {{if .User}} guards ".User"; {{if and .User ...}}
// guards ".User" too, because and evaluates left to right and stops at the
// first empty value.
func guardedPath(pipe *template.PipeNode) (string, bool) {
	if pipe == nil || len(pipe.Commands) != 1 {
		return "", false
	}
	cmd := pipe.Commands[0]
	if len(cmd.Args) == 0 {
		return "", false
	}
	if id, ok := cmd.Args[0].(*template.IdentifierNode); ok && id.Ident == "and" && len(cmd.Args) >= 2 {
		return fieldPathOf(cmd.Args[1])
	}
	if len(cmd.Args) != 1 {
		return "", false
	}
	return fieldPathOf(cmd.Args[0])
}

// fieldPathOf returns the dot-rooted path of a bare dot or field reference
// ("" for {{.}}, ".User.Name" for {{.User.Name}}).
func fieldPathOf(n template.Node) (string, bool) {
	switch a := n.(type) {
	case *template.DotNode:
		return "", true
	case *template.FieldNode:
		return "." + strings.Join(a.Ident, "."), true
	}
	return "", false
}

// literalCondition reports the constant truthiness of a branch pipeline when
// it is a single literal bool (so the branch it guards is dead), and whether
// the pipeline is such a literal at all. {{if true}} and {{if false}} qualify;
// anything with a function, field, or declaration does not. ({{if nil}} is
// already a type error — "nil is not a command" — so it never reaches here.)
func literalCondition(pipe *template.PipeNode) (truth, isLiteral bool) {
	if pipe == nil || len(pipe.Decl) > 0 || len(pipe.Commands) != 1 {
		return false, false
	}
	cmd := pipe.Commands[0]
	if len(cmd.Args) != 1 {
		return false, false
	}
	if b, ok := cmd.Args[0].(*template.BoolNode); ok {
		return b.True, true
	}
	return false, false
}
