package check

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"strconv"
	"strings"
	"text/template/parse"
)

type TreeFinder interface {
	FindTree(name string) (*parse.Tree, bool)
}

type FindTreeFunc func(name string) (*parse.Tree, bool)

func (fn FindTreeFunc) FindTree(name string) (*parse.Tree, bool) {
	return fn(name)
}

type FunctionFinder interface {
	FindFunction(name string) (*types.Signature, bool)
}

func Tree(tree *parse.Tree, data types.Type, pkg *types.Package, fileSet *token.FileSet, trees TreeFinder, functions FunctionFinder) error {
	s := &scope{
		global: global{
			TreeFinder:     trees,
			FunctionFinder: functions,
			pkg:            pkg,
			fileSet:        fileSet,
		},
		variables: map[string]types.Type{
			"$": data,
		},
	}
	_, err := s.walk(tree, data, tree.Root)
	return err
}

type global struct {
	TreeFinder
	FunctionFinder

	pkg     *types.Package
	fileSet *token.FileSet
}

type scope struct {
	global
	variables map[string]types.Type
}

func (s *scope) child() *scope {
	return &scope{
		global:    s.global,
		variables: maps.Clone(s.variables),
	}
}

func (s *scope) walk(tree *parse.Tree, dot types.Type, node parse.Node) (types.Type, error) {
	switch n := node.(type) {
	case *parse.DotNode:
		return dot, nil
	case *parse.ListNode:
		return nil, s.checkListNode(tree, dot, n)
	case *parse.ActionNode:
		return nil, s.checkActionNode(tree, dot, n)
	case *parse.CommandNode:
		return s.checkCommandNode(tree, dot, n)
	case *parse.FieldNode:
		return s.checkFieldNode(tree, dot, n)
	case *parse.PipeNode:
		return s.checkPipeNode(tree, dot, n)
	case *parse.IfNode:
		return nil, s.checkIfNode(tree, dot, n)
	case *parse.RangeNode:
		return nil, s.checkRangeNode(tree, dot, n)
	case *parse.TemplateNode:
		return nil, s.checkTemplateNode(tree, dot, n)
	case *parse.BoolNode:
		return types.Typ[types.UntypedBool], nil
	case *parse.StringNode:
		return types.Typ[types.UntypedString], nil
	case *parse.NumberNode:
		return newNumberNodeType(n)
	case *parse.VariableNode:
		return s.checkVariableNode(tree, n)
	case *parse.IdentifierNode:
		return s.checkIdentifierNode(n)
	case *parse.TextNode:
		return nil, nil
	case *parse.WithNode:
		return nil, s.checkWithNode(tree, dot, n)
	case *parse.CommentNode:
		return nil, nil
	case *parse.NilNode:
		return types.Typ[types.UntypedNil], nil
	case *parse.ChainNode:
		return s.checkChainNode(tree, dot, n)
	case *parse.BranchNode:
		return nil, nil
	case *parse.BreakNode:
		return nil, nil
	case *parse.ContinueNode:
		return nil, nil
	default:
		return nil, fmt.Errorf("missing node type check %T", n)
	}
}

func (s *scope) checkChainNode(tree *parse.Tree, dot types.Type, n *parse.ChainNode) (types.Type, error) {
	x, err := s.walk(tree, dot, n.Node)
	if err != nil {
		return nil, err
	}
	return s.checkIdentifiers(tree, x, n, n.Field)
}

func (s *scope) checkVariableNode(tree *parse.Tree, n *parse.VariableNode) (types.Type, error) {
	tp, ok := s.variables[n.Ident[0]]
	if !ok {
		return nil, fmt.Errorf("variable %s not found", n.Ident[0])
	}
	return s.checkIdentifiers(tree, tp, n, n.Ident[1:])
}

func (s *scope) checkListNode(tree *parse.Tree, dot types.Type, n *parse.ListNode) error {
	for _, child := range n.Nodes {
		if _, err := s.walk(tree, dot, child); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkActionNode(tree *parse.Tree, dot types.Type, n *parse.ActionNode) error {
	_, err := s.walk(tree, dot, n.Pipe)
	return err
}

func (s *scope) checkPipeNode(tree *parse.Tree, dot types.Type, n *parse.PipeNode) (types.Type, error) {
	x := dot
	for _, cmd := range n.Cmds {
		tp, err := s.walk(tree, x, cmd)
		if err != nil {
			return nil, err
		}
		x = tp
	}
	if len(n.Decl) > 0 {
		switch r := x.(type) {
		case *types.Slice:
			if l := len(n.Decl); l == 1 {
				s.variables[n.Decl[0].Ident[0]] = r.Elem()
			} else if l == 2 {
				s.variables[n.Decl[0].Ident[0]] = types.Typ[types.Int]
				s.variables[n.Decl[1].Ident[0]] = r.Elem()
			} else {
				return nil, fmt.Errorf("expected 1 or 2 declaration")
			}
		case *types.Array:
			if l := len(n.Decl); l == 1 {
				s.variables[n.Decl[0].Ident[0]] = r.Elem()
			} else if l == 2 {
				s.variables[n.Decl[0].Ident[0]] = types.Typ[types.Int]
				s.variables[n.Decl[1].Ident[0]] = r.Elem()
			} else {
				return nil, fmt.Errorf("expected 1 or 2 declaration")
			}
		case *types.Map:
			if l := len(n.Decl); l == 1 {
				s.variables[n.Decl[0].Ident[0]] = r.Elem()
			} else if l == 2 {
				s.variables[n.Decl[0].Ident[0]] = r.Key()
				s.variables[n.Decl[1].Ident[0]] = r.Elem()
			} else {
				return nil, fmt.Errorf("expected 1 or 2 declaration")
			}
		default:
			// assert.MaxLen(n.Decl, 1, "too many variable declarations in a pipe node")
			if len(n.Decl) == 1 {
				s.variables[n.Decl[0].Ident[0]] = x
			}
		}
	}
	return x, nil
}

func (s *scope) checkIfNode(tree *parse.Tree, dot types.Type, n *parse.IfNode) error {
	_, err := s.walk(tree, dot, n.Pipe)
	if err != nil {
		return err
	}
	ifScope := s.child()
	if _, err := ifScope.walk(tree, dot, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		elseScope := s.child()
		if _, err := elseScope.walk(tree, dot, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkWithNode(tree *parse.Tree, dot types.Type, n *parse.WithNode) error {
	child := s.child()
	x, err := child.walk(tree, dot, n.Pipe)
	if err != nil {
		return err
	}
	withScope := child.child()
	if _, err := withScope.walk(tree, x, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		elseScope := child.child()
		if _, err := elseScope.walk(tree, dot, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func newNumberNodeType(n *parse.NumberNode) (types.Type, error) {
	if n.IsInt || n.IsUint {
		tp := types.Typ[types.UntypedInt]
		return tp, nil
	}
	if n.IsFloat {
		tp := types.Typ[types.UntypedFloat]
		return tp, nil
	}
	if n.IsComplex {
		tp := types.Typ[types.UntypedComplex]
		return tp, nil
	}
	return nil, fmt.Errorf("failed to evaluate template *parse.NumberNode type")
}

func (s *scope) checkTemplateNode(tree *parse.Tree, dot types.Type, n *parse.TemplateNode) error {
	x := dot
	if n.Pipe != nil {
		tp, err := s.walk(tree, x, n.Pipe)
		if err != nil {
			return err
		}
		x = tp
		x = downgradeUntyped(x)
	} else {
		x = types.Typ[types.UntypedNil]
	}
	childTree, ok := s.FindTree(n.Name)
	if !ok {
		return fmt.Errorf("template %q not found", n.Name)
	}
	childScope := scope{
		global: s.global,
		variables: map[string]types.Type{
			"$": x,
		},
	}
	_, err := childScope.walk(childTree, x, childTree.Root)
	return err
}

func downgradeUntyped(x types.Type) types.Type {
	if x == nil {
		return x
	}
	basic, ok := x.Underlying().(*types.Basic)
	if !ok {
		return x
	}
	switch k := basic.Kind(); k {
	case types.UntypedInt:
		return types.Typ[types.Int].Underlying()
	case types.UntypedRune:
		return types.Typ[types.Rune].Underlying()
	case types.UntypedFloat:
		return types.Typ[types.Float64].Underlying()
	case types.UntypedComplex:
		return types.Typ[types.Complex128].Underlying()
	case types.UntypedString:
		return types.Typ[types.String].Underlying()
	default:
		return x
	}
}

func (s *scope) checkFieldNode(tree *parse.Tree, dot types.Type, n *parse.FieldNode) (types.Type, error) {
	return s.checkIdentifiers(tree, dot, n, n.Ident)
}

func (s *scope) checkCommandNode(tree *parse.Tree, dot types.Type, n *parse.CommandNode) (types.Type, error) {
	if _, ok := n.Args[0].(*parse.NilNode); len(n.Args) == 1 && ok {
		loc, _ := tree.ErrorContext(n)
		return nil, fmt.Errorf("%s: executing %q at <%s>: nil is not a command", loc, tree.Name, n.Args[0].String())
	}
	argTypes := make([]types.Type, 0, len(n.Args))
	for _, arg := range n.Args[1:] {
		argType, err := s.walk(tree, dot, arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, argType)
	}
	if ident, ok := n.Args[0].(*parse.IdentifierNode); ok {
		switch ident.Ident {
		case "slice":
			var result types.Type
			if slice, ok := argTypes[0].(*types.Slice); ok {
				result = slice.Elem()
			} else if array, ok := argTypes[0].(*types.Array); ok {
				result = array.Elem()
			}
			if len(argTypes) > 1 {
				first, ok := argTypes[1].(*types.Basic)
				if !ok {
					return nil, fmt.Errorf("slice expected int")
				}
				switch first.Kind() {
				case types.UntypedInt, types.Int:
				default:
				}
			}
			if len(argTypes) > 2 {
				second, ok := argTypes[1].(*types.Basic)
				if !ok {
					return nil, fmt.Errorf("slice expected int")
				}
				switch second.Kind() {
				case types.UntypedInt, types.Int:
				default:
				}
			}
			return result, nil
		case "index":
		}
	}
	cmdType, err := s.walk(tree, dot, n.Args[0])
	if err != nil {
		return nil, err
	}
	switch cmd := cmdType.(type) {
	case *types.Signature:
		for i := 0; i < len(argTypes); i++ {
			at := argTypes[i]
			var pt types.Type
			isVar := cmd.Variadic()
			argVar := i >= cmd.Params().Len()-1
			if isVar && argVar {
				ps := cmd.Params()
				v := ps.At(ps.Len() - 1).Type().(*types.Slice)
				pt = v.Elem()
			} else {
				pt = cmd.Params().At(i).Type()
			}
			assignable := types.AssignableTo(at, pt)
			if !assignable {
				return nil, fmt.Errorf("%s argument %d has type %s expected %s", n.Args[0], i, at, pt)
			}
		}
		return cmd.Results().At(0).Type(), nil
	default:
		return cmd, nil
	}
}

func (s *scope) checkIdentifiers(tree *parse.Tree, dot types.Type, n parse.Node, idents []string) (types.Type, error) {
	x := dot
	for i, ident := range idents {
		for {
			ptr, ok := x.(*types.Pointer)
			if !ok {
				break
			}
			x = ptr.Elem()
		}
		switch xx := x.(type) {
		case *types.Map:
			switch key := xx.Key().Underlying().(type) {
			case *types.Basic:
				switch key.Kind() {
				// case types.Int, types.Int64, types.Int32, types.Int16, types.Int8,
				//	types.Uint, types.Uint64, types.Uint32, types.Uint16, types.Uint8:
				case types.Int:
					x = xx.Elem()
					_, err := strconv.Atoi(ident)
					if err != nil {
						loc, _ := tree.ErrorContext(n)
						return nil, fmt.Errorf(`%s: executing %q at <%s>: can't evaluate field one in type %s`, loc, tree.Name, n.String(), xx.String())
					}
				case types.String:
					x = xx.Elem()
				default:
				}
				continue
			default:
				x = xx.Elem()
			}
			continue
		case *types.Named:
			obj, _, _ := types.LookupFieldOrMethod(x, true, nil, ident)
			if obj == nil {
				loc, _ := tree.ErrorContext(n)
				return nil, fmt.Errorf("type check failed: %s: %s not found on %s", loc, ident, x)
			}
			switch o := obj.(type) {
			default:
				x = obj.Type()
			case *types.Func:
				sig := o.Signature()
				resultLen := sig.Results().Len()
				if resultLen < 1 || resultLen > 2 {
					loc, _ := tree.ErrorContext(n)
					methodPos := s.fileSet.Position(o.Pos())
					return nil, fmt.Errorf("type check failed: %s: function %s has %d return values; should be 1 or 2: incorrect signature at %s", loc, ident, resultLen, methodPos)
				}
				if resultLen > 1 {
					loc, _ := tree.ErrorContext(n)
					methodPos := s.fileSet.Position(obj.Pos())
					finalResult := sig.Results().At(sig.Results().Len() - 1)
					errorType := types.Universe.Lookup("error")
					if !types.Identical(errorType.Type(), finalResult.Type()) {
						return nil, fmt.Errorf("type check failed: %s: invalid function signature for %s: second return value should be error; is %s: incorrect signature at %s", loc, ident, finalResult.Type(), methodPos)
					}
				}
				if i == len(idents)-1 {
					return o.Type(), nil
				}
				x = sig.Results().At(0).Type()
			}
			if _, ok := x.(*types.Signature); ok && i < len(idents)-1 {
				loc, _ := tree.ErrorContext(n)
				return nil, fmt.Errorf("type check failed: %s: can't evaluate field %s in type %s", loc, ident, x)
			}
		default:
			loc, _ := tree.ErrorContext(n)
			return nil, fmt.Errorf("type check failed: %s: identifier chain not supported for type %s", loc, x.String())
		}
	}
	return x, nil
}

func (s *scope) checkRangeNode(tree *parse.Tree, dot types.Type, n *parse.RangeNode) error {
	child := s.child()
	pipeType, err := child.walk(tree, dot, n.Pipe)
	if err != nil {
		return err
	}
	var x types.Type
	switch pt := pipeType.(type) {
	case *types.Slice:
		x = pt.Elem()
	case *types.Array:
		x = pt.Elem()
	case *types.Map:
		x = pt.Elem()
	default:
		return fmt.Errorf("failed to range over %s", pipeType)
	}
	if _, err := child.walk(tree, x, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		if _, err := child.walk(tree, x, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkIdentifierNode(n *parse.IdentifierNode) (types.Type, error) {
	if strings.HasPrefix(n.Ident, "$") {
		tp, ok := s.variables[n.Ident]
		if !ok {
			return nil, fmt.Errorf("failed to find identifier %s", n.Ident)
		}
		return tp, nil
	}
	fn, ok := s.FindFunction(n.Ident)
	if !ok {
		return nil, fmt.Errorf("failed to find function %s", n.Ident)
	}
	return fn, nil
}
