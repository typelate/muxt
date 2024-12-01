package check

import (
	"fmt"
	"go/token"
	"go/types"
	"text/template/parse"
)

type TreeFinder interface {
	FindTree(name string) (*parse.Tree, bool)
}

type FunctionFinder interface {
	FindFunction(name string) (*types.Signature, bool)
}

func Tree(tree *parse.Tree, data types.Type, pkg *types.Package, fileSet *token.FileSet, trees TreeFinder, functions FunctionFinder) error {
	s := &scope{
		TreeFinder:     trees,
		FunctionFinder: functions,
		pkg:            pkg,
		fileSet:        fileSet,
	}
	_, err := s.typeCheckNode(tree, data, tree.Root)
	return err
}

type scope struct {
	TreeFinder
	FunctionFinder

	pkg     *types.Package
	fileSet *token.FileSet
}

func (s *scope) typeCheckNode(tree *parse.Tree, dot types.Type, node parse.Node) (types.Type, error) {
	switch n := node.(type) {
	case *parse.DotNode:
		return dot, nil
	case *parse.ListNode:
		return s.checkListNode(tree, dot, n)
	case *parse.ActionNode:
		return s.checkActionNode(tree, dot, n)
	case *parse.CommandNode:
		return s.checkCommandNode(tree, dot, n)
	case *parse.FieldNode:
		return s.checkFieldNode(tree, dot, n)
	case *parse.PipeNode:
		return s.checkPipeNode(tree, dot, n)
	case *parse.IfNode:
		return s.checkIfNode(tree, dot, n)
	case *parse.TemplateNode:
		return s.checkTemplateNode(tree, dot, n)
	case *parse.BoolNode:
		return types.Typ[types.UntypedBool], nil
	case *parse.StringNode:
		return types.Typ[types.UntypedString], nil
	case *parse.NumberNode:
		return newNumberNodeType(n)
	case *parse.TextNode:
		return nil, nil
	default:
		return nil, fmt.Errorf("missing node type check %T", n)
	}
}

func (s *scope) checkListNode(tree *parse.Tree, dot types.Type, n *parse.ListNode) (types.Type, error) {
	for _, child := range n.Nodes {
		if _, err := s.typeCheckNode(tree, dot, child); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (s *scope) checkActionNode(tree *parse.Tree, dot types.Type, n *parse.ActionNode) (types.Type, error) {
	for _, cmd := range n.Pipe.Cmds {
		if _, err := s.typeCheckNode(tree, dot, cmd); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (s *scope) checkPipeNode(tree *parse.Tree, dot types.Type, n *parse.PipeNode) (types.Type, error) {
	x := dot
	for _, cmd := range n.Cmds {
		tp, err := s.typeCheckNode(tree, x, cmd)
		if err != nil {
			return nil, err
		}
		x = tp
	}
	return x, nil
}

func (s *scope) checkIfNode(tree *parse.Tree, dot types.Type, n *parse.IfNode) (types.Type, error) {
	tp, err := s.typeCheckNode(tree, dot, n.Pipe)
	if err != nil {
		return nil, err
	}
	return tp, nil
}

func newNumberNodeType(n *parse.NumberNode) (types.Type, error) {
	if n.IsInt {
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

func (s *scope) checkTemplateNode(tree *parse.Tree, dot types.Type, n *parse.TemplateNode) (types.Type, error) {
	x := dot
	if n.Pipe != nil {
		tp, err := s.typeCheckNode(tree, x, n.Pipe)
		if err != nil {
			return nil, err
		}
		x = tp
	} else {
		x = types.Typ[types.UntypedNil]
	}
	child, ok := s.FindTree(n.Name)
	if !ok {
		return nil, fmt.Errorf("template %q not found", n.Name)
	}
	return s.typeCheckNode(child, x, n.Pipe)
}

func (s *scope) checkFieldNode(tree *parse.Tree, dot types.Type, n *parse.FieldNode) (types.Type, error) {
	x := dot
	for i, ident := range n.Ident {
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
			if i == len(n.Ident)-1 {
				return o.Type(), nil
			}
			x = sig.Results().At(0).Type()
		}
		if _, ok := x.(*types.Signature); ok && i < len(n.Ident)-1 {
			loc, _ := tree.ErrorContext(n)
			return nil, fmt.Errorf("type check failed: %s: can't evaluate field %s in type %s", loc, ident, x)
		}
	}
	return x, nil
}

func (s *scope) checkCommandNode(tree *parse.Tree, dot types.Type, n *parse.CommandNode) (types.Type, error) {
	var argTypes []types.Type
	for _, arg := range n.Args {
		argType, err := s.typeCheckNode(tree, dot, arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, argType)
	}
	if len(n.Args) > 1 {
		cmd := argTypes[0]
		argTypes = argTypes[1:]

		sig := cmd.(*types.Signature)

		for i := 0; i < len(argTypes); i++ {
			at := argTypes[i]
			pt := sig.Params().At(i).Type()
			if !types.AssignableTo(at, pt) {
				return nil, fmt.Errorf("%s argument %d has type %s expected %s", n.Args[0], i-1, at, pt)
			}
		}
	}
	return nil, nil
}
