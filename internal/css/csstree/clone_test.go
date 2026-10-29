// Ported from lib/__tests/clone.js.

package csstree

import (
	"reflect"
	"slices"
	"testing"
)

func createCloneTest(t *testing.T, name string, getAST func(t *testing.T) Node) {
	t.Run(name, func(t *testing.T) {
		ast := getAST(t)
		astCopy := Clone(ast)
		var astNodes []Node
		clonedNodeCount := 0
		nonClonedNodeCount := 0

		mustWalk(t, ast, enterOnly(func(node Node) { astNodes = append(astNodes, node) }))
		mustWalk(t, astCopy, enterOnly(func(node Node) {
			clonedNodeCount++
			if slices.Contains(astNodes, node) {
				nonClonedNodeCount++
			}
		}))

		if clonedNodeCount != len(astNodes) {
			t.Errorf("Clone() has %d nodes, want %d", clonedNodeCount, len(astNodes))
		}
		if nonClonedNodeCount != 0 {
			t.Errorf("Clone() shares %d nodes with the original, want 0", nonClonedNodeCount)
		}

		// Go only: the copy is equal to the original
		if got, want := toPlainJSON(t, astCopy), toPlainJSON(t, ast); !reflect.DeepEqual(got, want) {
			t.Errorf("Clone() = %v, want %v", got, want)
		}
	})
}

func TestClone(t *testing.T) {
	t.Run("clone()", func(t *testing.T) {
		createCloneTest(t, "a regular AST", func(t *testing.T) Node {
			return mustParse(t, ".test{color:red;}@media foo{div{color:green}}", ParseOptions{})
		})

		// Upstream clones the plain object itself; a Go plain object is
		// maps and slices, so this clones the AST built back from it.
		createCloneTest(t, "an AST as JSON", func(t *testing.T) Node {
			plainObject, err := ToPlainObject(mustParse(t, ".test{color:red;}@media foo{div{color:green}}", ParseOptions{}))
			if err != nil {
				t.Fatal(err)
			}
			ast, err := FromPlainObject(plainObject)
			if err != nil {
				t.Fatal(err)
			}
			return ast
		})

		// Go only: locations are copied too
		t.Run("copies locations", func(t *testing.T) {
			ast := mustParse(t, ".a{}", ParseOptions{Positions: true}).(*StyleSheet)
			astCopy := Clone(ast).(*StyleSheet)

			if astCopy.Loc == nil || astCopy.Loc == ast.Loc || *astCopy.Loc != *ast.Loc {
				t.Errorf("Clone().Loc = %v, want a copy of %v", astCopy.Loc, ast.Loc)
			}
		})
	})
}
