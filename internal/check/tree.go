package check

import (
	"fmt"
	"go/token"
	"go/types"
	"text/template/parse"
)

func Tree(tree *parse.Tree, tp types.Type, pkg *types.Package, fileSet *token.FileSet, templates map[string]*parse.Tree, funcs map[string]*types.Signature) error {
	return typeCheckNode(tree, tp, pkg, fileSet, templates, funcs, tree.Root)
}

func typeCheckNode(tree *parse.Tree, tp types.Type, pkg *types.Package, fileSet *token.FileSet, templates map[string]*parse.Tree, funcs map[string]*types.Signature, node parse.Node) error {
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			if err := typeCheckNode(tree, tp, pkg, fileSet, templates, funcs, child); err != nil {
				return err
			}
		}
		return nil
	case *parse.ActionNode:
		for _, cmd := range n.Pipe.Cmds {
			if err := typeCheckNode(tree, tp, pkg, fileSet, templates, funcs, cmd); err != nil {
				return err
			}
		}
		return nil
	case *parse.CommandNode:
		for _, arg := range n.Args {
			if err := typeCheckNode(tree, tp, pkg, fileSet, templates, funcs, arg); err != nil {
				return err
			}
		}
		return nil
	case *parse.FieldNode:
		x := tp
		for _, ident := range n.Ident {
			obj, _, _ := types.LookupFieldOrMethod(x, true, pkg, ident)
			if obj == nil {
				loc, _ := tree.ErrorContext(n)
				return fmt.Errorf("type check failed: %s: %s not found on %s", loc, ident, x)
			}
			switch o := obj.Type().(type) {
			case *types.Signature:
				if o.Recv() == nil {
					loc, _ := tree.ErrorContext(n)
					return fmt.Errorf("type check failed: %s: can't evaluate field %s in type %s", loc, ident, obj.Type())
				}
				resultLen := o.Results().Len()
				if resultLen < 1 || resultLen > 2 {
					loc, _ := tree.ErrorContext(n)
					methodPos := fileSet.Position(obj.Pos())
					return fmt.Errorf("type check failed: %s: function %s has %d return values; should be 1 or 2: incorrect signature at %s", loc, ident, resultLen, methodPos)
				}
				if resultLen > 1 {
					loc, _ := tree.ErrorContext(n)
					methodPos := fileSet.Position(obj.Pos())
					finalResult := o.Results().At(o.Results().Len() - 1)
					errorType := types.Universe.Lookup("error")
					if !types.Identical(errorType.Type(), finalResult.Type()) {
						return fmt.Errorf("type check failed: %s: invalid function signature for %s: second return value should be error; is %s: incorrect signature at %s", loc, ident, finalResult.Type(), methodPos)
					}
				}
				x = o.Results().At(0).Type()
			default:
				x = obj.Type()
			}
		}
		return nil
	case *parse.DotNode:
		return nil
	default:
		return fmt.Errorf("missing node type check %T", n)
	}
}
