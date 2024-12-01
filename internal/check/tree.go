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
	_, err := typeCheckNode(tree, data, s, tree.Root)
	return err
}

type scope struct {
	TreeFinder
	FunctionFinder

	pkg     *types.Package
	fileSet *token.FileSet
}

func typeCheckNode(tree *parse.Tree, dot types.Type, parent *scope, node parse.Node) (types.Type, error) {
	switch n := node.(type) {
	case *parse.DotNode:
		return dot, nil
	case *parse.ListNode:
		return checkListNode(tree, dot, parent, n)
	case *parse.ActionNode:
		return checkActionNode(tree, dot, parent, n)
	case *parse.CommandNode:
		return checkCommandNode(tree, dot, parent, n)
	case *parse.FieldNode:
		return checkFieldNode(tree, dot, parent.fileSet, n)
	case *parse.PipeNode:
		return checkPipeNode(tree, dot, parent, n)
	case *parse.IfNode:
		return checkIfNode(tree, dot, parent, n)
	case *parse.TemplateNode:
		return checkTemplateNode(tree, dot, parent, n)
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

func checkListNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.ListNode) (types.Type, error) {
	for _, child := range n.Nodes {
		if _, err := typeCheckNode(tree, dot, s, child); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func checkActionNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.ActionNode) (types.Type, error) {
	for _, cmd := range n.Pipe.Cmds {
		if _, err := typeCheckNode(tree, dot, s, cmd); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func checkPipeNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.PipeNode) (types.Type, error) {
	x := dot
	for _, cmd := range n.Cmds {
		tp, err := typeCheckNode(tree, x, s, cmd)
		if err != nil {
			return nil, err
		}
		x = tp
	}
	return x, nil
}

func checkIfNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.IfNode) (types.Type, error) {
	tp, err := typeCheckNode(tree, dot, s, n.Pipe)
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

func checkTemplateNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.TemplateNode) (types.Type, error) {
	x := dot
	if n.Pipe != nil {
		tp, err := typeCheckNode(tree, x, s, n.Pipe)
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
	return typeCheckNode(child, x, s, n.Pipe)
}

func checkFieldNode(tree *parse.Tree, dot types.Type, fileSet *token.FileSet, n *parse.FieldNode) (types.Type, error) {
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
				methodPos := fileSet.Position(o.Pos())
				return nil, fmt.Errorf("type check failed: %s: function %s has %d return values; should be 1 or 2: incorrect signature at %s", loc, ident, resultLen, methodPos)
			}
			if resultLen > 1 {
				loc, _ := tree.ErrorContext(n)
				methodPos := fileSet.Position(obj.Pos())
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

func checkCommandNode(tree *parse.Tree, dot types.Type, s *scope, n *parse.CommandNode) (types.Type, error) {
	var argTypes []types.Type
	for _, arg := range n.Args {
		argType, err := typeCheckNode(tree, dot, s, arg)
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
