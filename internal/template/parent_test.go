package template

import (
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

func parseForParent(t *testing.T, src string) *Tree {
	t.Helper()
	tree, err := ParseTree("parent", "parent", src, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
		BreakOK:    true,
		ContinueOK: true,
	})
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return tree
}

func firstChildElement(children []Node) *HTMLElementNode {
	for _, c := range children {
		if el, ok := c.(*HTMLElementNode); ok {
			return el
		}
	}
	return nil
}

func TestNodeParent(t *testing.T) {
	tree := parseForParent(t, `<html><body><p>hi</p></body></html>`+
		`{{define "frag"}}<span class="x"></span>{{end}}`)

	root := tree.Root
	frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode)
	if !ok {
		t.Fatalf("root.Nodes[0] is %T, want *HTMLDocumentFragmentNode", root.Nodes[0])
	}

	t.Run("root has no parent", func(t *testing.T) {
		if got := root.Parent(); got != nil {
			t.Errorf("root.Parent() = %v, want nil", got)
		}
		if got := frag.Parent(); got != Node(root) {
			t.Errorf("fragment.Parent() = %v, want the root list", got)
		}
	})

	htmlEl := firstChildElement(frag.Children)
	bodyEl := firstChildElement(htmlEl.Children)
	pEl := firstChildElement(bodyEl.Children)

	t.Run("element parent chain", func(t *testing.T) {
		if got := htmlEl.Parent(); got != Node(frag) {
			t.Errorf("<html>.Parent() = %v, want the document fragment", got)
		}
		if got := bodyEl.Parent(); got != Node(htmlEl) {
			t.Errorf("<body>.Parent() = %v, want <html>", got)
		}
		if got := pEl.Parent(); got != Node(bodyEl) {
			t.Errorf("<p>.Parent() = %v, want <body>", got)
		}
	})

	t.Run("define is a detached root, not parented to the document", func(t *testing.T) {
		var def *DefineNode
		for _, c := range frag.Children {
			if d, ok := c.(*DefineNode); ok {
				def = d
			}
		}
		if def == nil {
			t.Fatal("no DefineNode among document children")
		}
		if got := def.Parent(); got != nil {
			t.Errorf("define.Parent() = %v, want nil (detached root)", got)
		}
		if got := def.Body.Parent(); got != Node(def) {
			t.Errorf("define body.Parent() = %v, want the define node", got)
		}
		span := firstChildElement(def.Body.Nodes)
		if span == nil {
			t.Fatal("no element in define body")
		}
		if got := span.Parent(); got != Node(def.Body) {
			t.Errorf("<span>.Parent() = %v, want the define body list", got)
		}
	})
}
