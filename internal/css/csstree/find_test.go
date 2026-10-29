// Ported from lib/__tests/find.js.

package csstree

import "testing"

func TestFind(t *testing.T) {
	ast := mustParse(t, `
        .foo { color: red; background: green; }
        .bar, .qux.foo { font-weight: bold; color: blue; }
    `, ParseOptions{})
	firstFoo := ast.(*StyleSheet).
		Children.First().(*Rule).     // Rule
		Prelude.(*SelectorList).      // SelectorList
		Children.First().(*Selector). // Selector
		Children.First()              // ClassSelector
	lastFoo := ast.(*StyleSheet).
		Children.Last().(*Rule).     // Rule
		Prelude.(*SelectorList).     // SelectorList
		Children.Last().(*Selector). // Selector
		Children.Last()              // ClassSelector

	isFoo := func(node Node) bool {
		classSelector, ok := node.(*ClassSelector)
		return ok && classSelector.Name == "foo"
	}
	selectorChildren := func(context *WalkContext) *List[Node] {
		return context.Selector.(*SelectorList).Children
	}

	t.Run("Search", func(t *testing.T) {
		t.Run("find", func(t *testing.T) {
			t.Run("base", func(t *testing.T) {
				actual := Find(ast, func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) bool {
					return isFoo(node)
				})

				if actual != firstFoo {
					t.Errorf("Find(ast, .foo) = %v, want the first .foo", actual)
				}
			})

			t.Run("using refs", func(t *testing.T) {
				actual := Find(ast, func(_ *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool {
					return isFoo(node) && list.Head() != item
				})

				if actual != lastFoo {
					t.Errorf("Find(ast, .foo not first) = %v, want the last .foo", actual)
				}
			})

			t.Run("using context", func(t *testing.T) {
				actual := Find(ast, func(context *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) bool {
					return isFoo(node) &&
						selectorChildren(context).Head() != selectorChildren(context).Tail()
				})

				if actual != lastFoo {
					t.Errorf("Find(ast, .foo in a selector list of several) = %v, want the last .foo", actual)
				}
			})
		})

		t.Run("findLast", func(t *testing.T) {
			t.Run("findLast", func(t *testing.T) {
				actual := FindLast(ast, func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) bool {
					return isFoo(node)
				})

				if actual != lastFoo {
					t.Errorf("FindLast(ast, .foo) = %v, want the last .foo", actual)
				}
			})

			t.Run("using refs", func(t *testing.T) {
				actual := FindLast(ast, func(_ *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool {
					return isFoo(node) && list.Head() == item
				})

				if actual != firstFoo {
					t.Errorf("FindLast(ast, .foo first) = %v, want the first .foo", actual)
				}
			})

			t.Run("using context", func(t *testing.T) {
				actual := FindLast(ast, func(context *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) bool {
					return isFoo(node) &&
						selectorChildren(context).Head() == selectorChildren(context).Tail()
				})

				if actual != firstFoo {
					t.Errorf("FindLast(ast, .foo in a single selector list) = %v, want the first .foo", actual)
				}
			})
		})

		t.Run("findAll", func(t *testing.T) {
			actual := FindAll(ast, func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) bool {
				return isFoo(node)
			})

			if len(actual) != 2 {
				t.Fatalf("len(FindAll(ast, .foo)) = %d, want 2", len(actual))
			}
			if actual[0] != firstFoo {
				t.Errorf("FindAll(ast, .foo)[0] = %v, want the first .foo", actual[0])
			}
			if actual[1] != lastFoo {
				t.Errorf("FindAll(ast, .foo)[1] = %v, want the last .foo", actual[1])
			}
		})
	})
}
