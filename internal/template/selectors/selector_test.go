package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
	"github.com/typelate/muxt/internal/template/selectors"
)

// parseHTML parses HTML+template source into a tree with HTML contexts annotated.
func parseHTML(t *testing.T, src string) *template.Tree {
	t.Helper()
	tree, err := template.ParseTree("test", "test", src, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
		BreakOK:    true,
		ContinueOK: true,
	})
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	template.AnnotateHTMLContexts(tree.Root)
	return tree
}

// firstElement returns the first HTMLElementNode with the given tag name, or
// the first element of any tag when tag is empty.
func firstElement(t *testing.T, tree *template.Tree, tag string) *template.HTMLElementNode {
	t.Helper()
	var found *template.HTMLElementNode
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if found != nil {
			return
		}
		if el, ok := n.(*template.HTMLElementNode); ok && (tag == "" || el.TagName == tag) {
			found = el
		}
	})
	if found == nil {
		t.Fatalf("no <%s> element found", tag)
	}
	return found
}

func TestSelectorMatch_TypeAndUniversal(t *testing.T) {
	tree := parseHTML(t, `<div><span></span></div>`)
	div := firstElement(t, tree, "div")
	span := firstElement(t, tree, "span")

	for _, tt := range []struct {
		name     string
		selector string
		node     template.Node
		want     selectors.Trit
	}{
		{"type selector matches its tag", "div", div, selectors.TritYes},
		{"type selector rejects another tag", "div", span, selectors.TritNo},
		{"universal matches an element", "*", div, selectors.TritYes},
		{"universal matches another element", "*", span, selectors.TritYes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sel := selectors.MustCompile(tt.selector)
			if _, got := sel.Matches(tt.node); got != tt.want {
				t.Errorf("MustCompile(%q).Match(<%s>) = %s, want %s",
					tt.selector, tt.node.(*template.HTMLElementNode).TagName, got, tt.want)
			}
		})
	}
}
