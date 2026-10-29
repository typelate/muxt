// Ported from lib/definition-syntax/walk.js.

package definitionsyntax

import "errors"

// WalkOptions holds the walk handlers. Upstream also accepts a single
// function, which is Enter here, and a context for `this`, which Go closures
// make unnecessary.
type WalkOptions struct {
	// Enter is called for a node before its children.
	Enter func(node Node)
	// Leave is called for a node after its children.
	Leave func(node Node)
}

func noopHandler(Node) {}

// Walk visits node and its descendants depth first. It returns an error when
// neither handler is set, or when it meets a nil node.
func Walk(node Node, options WalkOptions) error {
	enter, leave := options.Enter, options.Leave

	if enter == nil && leave == nil {
		//lint:ignore ST1005 upstream message
		return errors.New("Neither `enter` nor `leave` walker handler is set or both aren't a function")
	}

	if enter == nil {
		enter = noopHandler
	}
	if leave == nil {
		leave = noopHandler
	}

	var walk func(node Node) error
	walk = func(node Node) error {
		enter(node)

		switch node := node.(type) {
		case *Group:
			for _, term := range node.Terms {
				if err := walk(term); err != nil {
					return err
				}
			}

		case *Multiplier:
			if err := walk(node.Term); err != nil {
				return err
			}

		case *Boolean:
			if err := walk(node.Term); err != nil {
				return err
			}

		case *Type, *Property, *Keyword, *AtKeyword, *Function, *String, *Token, *Comma:

		default:
			nodeType := "<nil>"
			if node != nil {
				nodeType = node.Type()
			}
			return errors.New("Unknown type: " + nodeType)
		}

		leave(node)
		return nil
	}

	return walk(node)
}
