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

func Tree(tree *parse.Tree, data types.Type, pkg *types.Package, fileSet *token.FileSet, forrest TreeFinder, funcs map[string]*types.Signature) error {
	_, err := typeCheckNode(tree, data, pkg, fileSet, forrest, funcs, tree.Root)
	return err
}

func typeCheckNode(tree *parse.Tree, dot types.Type, pkg *types.Package, fileSet *token.FileSet, forrest TreeFinder, funcs map[string]*types.Signature, node parse.Node) (types.Type, error) {
	switch n := node.(type) {
	case *parse.DotNode:
		return dot, nil
	case *parse.ListNode:
		for _, child := range n.Nodes {
			if _, err := typeCheckNode(tree, dot, pkg, fileSet, forrest, funcs, child); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *parse.ActionNode:
		for _, cmd := range n.Pipe.Cmds {
			if _, err := typeCheckNode(tree, dot, pkg, fileSet, forrest, funcs, cmd); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *parse.CommandNode:
		var argTypes []types.Type
		for _, arg := range n.Args {
			argType, err := typeCheckNode(tree, dot, pkg, fileSet, forrest, funcs, arg)
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
	case *parse.FieldNode:
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
	case *parse.PipeNode:
		x := dot
		for _, cmd := range n.Cmds {
			tp, err := typeCheckNode(tree, x, pkg, fileSet, forrest, funcs, cmd)
			if err != nil {
				return nil, err
			}
			x = tp
		}
		return x, nil
	case *parse.IfNode:
		tp, err := typeCheckNode(tree, dot, pkg, fileSet, forrest, funcs, n.Pipe)
		if err != nil {
			return nil, err
		}
		return tp, nil
	case *parse.TemplateNode:
		x := dot
		if n.Pipe != nil {
			tp, err := typeCheckNode(tree, x, pkg, fileSet, forrest, funcs, n.Pipe)
			if err != nil {
				return nil, err
			}
			x = tp
		} else {
			x = types.Typ[types.UntypedNil]
		}
		child, ok := forrest.FindTree(n.Name)
		if !ok {
			return nil, fmt.Errorf("template %q not found", n.Name)
		}
		return typeCheckNode(child, x, pkg, fileSet, forrest, funcs, n.Pipe)
	case *parse.BoolNode:
		tp := types.Typ[types.UntypedBool]
		return tp, nil
	case *parse.StringNode:
		tp := types.Typ[types.UntypedString]
		return tp, nil
	case *parse.NumberNode:
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
	case *parse.TextNode:
		return nil, nil
	default:
		return nil, fmt.Errorf("missing node type check %T", n)
	}
}
