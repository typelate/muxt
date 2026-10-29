package selectors_test

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

// elementByID finds the element whose static id attribute equals id.
func elementByID(t *testing.T, tree *template.Tree, id string) *template.HTMLElementNode {
	t.Helper()
	var found *template.HTMLElementNode
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if found != nil {
			return
		}
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		for _, a := range el.Attr {
			attr, ok := a.(*template.HTMLAttributeNode)
			if !ok || len(attr.Key) != 1 || len(attr.Value) != 1 {
				continue
			}
			k, kok := attr.Key[0].(*template.TextNode)
			v, vok := attr.Value[0].(*template.TextNode)
			if kok && vok && string(k.Text) == "id" && strings.Trim(string(v.Text), `"'`) == id {
				found = el
			}
		}
	})
	if found == nil {
		t.Fatalf("no element #%s", id)
	}
	return found
}

func TestSelectorMatch_Combinators(t *testing.T) {
	tree := parseHTML(t, `<ul id="list"><li id="a">a</li>{{if .X}}<li id="b">b</li>{{end}}<li id="c">c</li></ul>`+
		`<section id="sec"><article id="art"><h2 id="h">t</h2><p id="p">x</p></article></section>`)

	for _, tt := range []struct {
		name     string
		selector string
		nodeID   string
		want     selectors.Trit
	}{
		{"descendant ancestor present", "ul li", "a", selectors.TritYes},
		{"descendant no such ancestor", "ul p", "p", selectors.TritNo},
		{"descendant deeper ancestor", "section p", "p", selectors.TritYes},

		{"child direct parent", "ul > li", "a", selectors.TritYes},
		{"child wrong parent", "section > h2", "h", selectors.TritNo},
		{"child right parent", "article > h2", "h", selectors.TritYes},

		{"adjacent certain predecessor", "h2 + p", "p", selectors.TritYes},
		{"adjacent subject mismatch", "h2 + p", "h", selectors.TritNo},
		{"adjacent through conditional is maybe", "li + li", "c", selectors.TritMaybe},

		{"general sibling certain", "#a ~ li", "c", selectors.TritYes},
		{"general sibling none before", "li ~ li", "a", selectors.TritNo},
		{"general sibling conditional is maybe", "#b ~ li", "c", selectors.TritMaybe},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node := elementByID(t, tree, tt.nodeID)
			sel := selectors.MustCompile(tt.selector)
			if _, got := sel.Matches(node); got != tt.want {
				t.Errorf("MustCompile(%q).Match(#%s) = %s, want %s",
					tt.selector, tt.nodeID, got, tt.want)
			}
		})
	}
}
