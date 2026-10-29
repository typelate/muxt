package template

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestWalkPath(t *testing.T) {
	tree, err := ParseTree("test.gohtml", "walkpath", `<table>{{if .Show}}<div>x</div>{{end}}</table>`, nil, lex.NewOptions())
	require.NoError(t, err)

	var divPath []string
	WalkPath(tree, func(_ *Tree, path []Node, n Node) {
		if el, ok := n.(*HTMLElementNode); ok && el.TagName == "div" {
			for _, p := range path {
				divPath = append(divPath, nodeTypeName(p))
			}
		}
	})
	// Root list > fragment > table > if > list > div: branch and list
	// nodes are present in the path so checkers can see control flow.
	assert.Equal(t, []string{"*template.ListNode", "*template.HTMLDocumentFragmentNode", "*template.HTMLElementNode", "*template.IfNode", "*template.ListNode"}, divPath)
}

func nodeTypeName(n Node) string { return fmt.Sprintf("%T", n) }

func TestWalkPathVisitsAttributeSubtrees(t *testing.T) {
	tree, err := ParseTree("test.gohtml", "walkattr", `<a href="{{.URL}}">x</a>`, nil, lex.NewOptions())
	require.NoError(t, err)
	sawActionUnderAttr := false
	WalkPath(tree, func(_ *Tree, path []Node, n Node) {
		if _, ok := n.(*ActionNode); ok {
			for _, p := range path {
				if _, ok := p.(*HTMLAttributeNode); ok {
					sawActionUnderAttr = true
				}
			}
		}
	})
	assert.True(t, sawActionUnderAttr, "action inside href must have HTMLAttributeNode in its path")
}

func TestWalkIsWrapperOverWalkPath(t *testing.T) {
	tree, err := ParseTree("test.gohtml", "wrap", `<p>{{.X}}</p>`, nil, lex.NewOptions())
	require.NoError(t, err)
	var a, b int
	Walk(tree, func(*Tree, Node) { a++ })
	WalkPath(tree, func(*Tree, []Node, Node) { b++ })
	assert.Equal(t, a, b, "Walk and WalkPath must visit identical node sets")
}
