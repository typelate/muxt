package check

import (
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func parseTestTree(t *testing.T, text string) *template.Tree {
	t.Helper()
	// Parse without HTML mode so each action is a direct child of the root.
	opts := lex.NewOptions()
	opts.ParseHTML = false
	tree, err := template.ParseTree("test.gohtml", "test.gohtml", text, nil, opts)
	require.NoError(t, err)
	return tree
}

func TestError_Unwrap(t *testing.T) {
	tree := parseTestTree(t, "{{.Name}}{{.Age}}")
	nodeA := tree.Root.Nodes[0]
	nodeB := tree.Root.Nodes[1]

	t.Run("leaf exposes its cause", func(t *testing.T) {
		cause := errors.New("boom")
		leaf := wrapError(ErrorTypeUnknown, tree, nodeA, cause)

		unwrapped := leaf.Unwrap()
		require.Len(t, unwrapped, 1)
		require.Same(t, cause, unwrapped[0])
		require.ErrorIs(t, leaf, cause)
	})

	t.Run("aggregate exposes children", func(t *testing.T) {
		causeA := errors.New("first failure")
		causeB := errors.New("second failure")
		childA := wrapError(ErrorTypeUnknown, tree, nodeA, causeA)
		childB := wrapError(ErrorTypeUnknown, tree, nodeB, causeB)

		agg, ok := joinErrors(tree, tree.Root, childA, childB).(*Error)
		require.True(t, ok, "joinErrors should return a *Error")

		unwrapped := agg.Unwrap()
		require.Len(t, unwrapped, 2)
		require.Same(t, childA, unwrapped[0])
		require.Same(t, childB, unwrapped[1])
		require.ErrorIs(t, agg, causeA)
		require.ErrorIs(t, agg, causeB)
	})
}

func TestError_Error(t *testing.T) {
	tree := parseTestTree(t, "{{.Name}}{{.Age}}")
	nodeA := tree.Root.Nodes[0]
	nodeB := tree.Root.Nodes[1]

	t.Run("aggregate joins child messages with newlines", func(t *testing.T) {
		childA := newError(ErrorTypeUnknown, tree, nodeA, "first failure")
		childB := newError(ErrorTypeUnknown, tree, nodeB, "second failure")

		agg := joinErrors(tree, tree.Root, childA, childB)
		require.Equal(t, childA.Error()+"\n"+childB.Error(), agg.Error())
	})

	t.Run("no tree renders the bare cause", func(t *testing.T) {
		e := newError(ErrorTypeUnknown, nil, nil, "boom")
		require.Equal(t, "boom", e.Error())
	})
}

func TestJoinErrors(t *testing.T) {
	tree := parseTestTree(t, "{{.Name}}{{.Age}}")
	nodeA := tree.Root.Nodes[0]

	t.Run("no errors returns nil", func(t *testing.T) {
		require.NoError(t, joinErrors(tree, tree.Root))
		require.NoError(t, joinErrors(tree, tree.Root, nil, nil))
	})

	t.Run("single error is returned unchanged", func(t *testing.T) {
		leaf := newError(ErrorTypeUnknown, tree, nodeA, "boom")
		require.Same(t, leaf, joinErrors(tree, tree.Root, nil, leaf))
	})

	t.Run("non-Error input is promoted to a leaf", func(t *testing.T) {
		cause := errors.New("boom")
		agg, ok := joinErrors(tree, tree.Root, cause, newError(ErrorTypeUnknown, tree, nodeA, "bam")).(*Error)
		require.True(t, ok)
		children := agg.Unwrap()
		require.Len(t, children, 2)
		var leaf *Error
		require.ErrorAs(t, children[0], &leaf)
		require.ErrorIs(t, children[0], cause)
	})
}

var _ iter.Seq[*Error] = (*Error)(nil).All

func TestError_All(t *testing.T) {
	tree := parseTestTree(t, "{{.Name}}{{.Age}}{{.Email}}")
	nodeA := tree.Root.Nodes[0]
	nodeB := tree.Root.Nodes[1]
	nodeC := tree.Root.Nodes[2]

	leafA := newError(ErrorTypeUnknown, tree, nodeA, "first failure")
	leafB := newError(ErrorTypeUnknown, tree, nodeB, "second failure")
	leafC := newError(ErrorTypeUnknown, tree, nodeC, "third failure")
	inner := joinErrors(tree, tree.Root, leafA, leafB).(*Error)
	root := joinErrors(tree, tree.Root, inner, leafC).(*Error)

	t.Run("walks the tree depth first", func(t *testing.T) {
		var got []*Error
		for e := range root.All {
			got = append(got, e)
		}
		require.Equal(t, []*Error{root, inner, leafA, leafB, leafC}, got)
	})

	t.Run("stops when yield returns false", func(t *testing.T) {
		var got []*Error
		for e := range root.All {
			got = append(got, e)
			if e == leafA {
				break
			}
		}
		require.Equal(t, []*Error{root, inner, leafA}, got)
	})

	t.Run("a leaf yields just itself", func(t *testing.T) {
		var got []*Error
		for e := range leafC.All {
			got = append(got, e)
		}
		require.Equal(t, []*Error{leafC}, got)
	})
}

func TestFormatVerbose_Error(t *testing.T) {
	tree := parseTestTree(t, "{{.Name}}{{.Age}}")
	nodeA := tree.Root.Nodes[0]
	nodeB := tree.Root.Nodes[1]

	t.Run("leaf keeps its location prefix", func(t *testing.T) {
		leaf := newError(ErrorTypeUnknown, tree, nodeA, "boom")
		require.Equal(t, leaf.VerboseError(), FormatVerbose(leaf))
	})

	t.Run("aggregate renders one block per child", func(t *testing.T) {
		childA := newError(ErrorTypeUnknown, tree, nodeA, "first failure")
		childB := newError(ErrorTypeUnknown, tree, nodeB, "second failure")

		agg := joinErrors(tree, tree.Root, childA, childB)
		require.Equal(t, childA.VerboseError()+"\n\n"+childB.VerboseError(), FormatVerbose(agg))
	})
}
