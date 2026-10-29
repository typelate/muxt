package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

func TestSelectorMatch_CompoundAndList(t *testing.T) {
	tree := parseHTML(t, `<a id="home" class="nav active" href="/"></a>`+
		`<span class="nav"></span>`+
		`<b id="{{.ID}}" class="card {{.Extra}}"></b>`)
	a := firstElement(t, tree, "a")
	span := firstElement(t, tree, "span")
	b := firstElement(t, tree, "b")

	for _, tt := range []struct {
		name     string
		selector string
		node     *template.HTMLElementNode
		want     selectors.Trit
	}{
		// compound: every simple must match the same element (AND)
		{"compound type+class hit", "a.nav", a, selectors.TritYes},
		{"compound type mismatch", "a.nav", span, selectors.TritNo},
		{"compound other type", "span.nav", span, selectors.TritYes},
		{"compound type+id", "a#home", a, selectors.TritYes},
		{"compound class miss", "a.missing", a, selectors.TritNo},
		{"compound maybe via class", "b.other", b, selectors.TritMaybe},

		// selector list: any alternative may match (OR)
		{"list first matches", "a, span", a, selectors.TritYes},
		{"list second matches", "a, span", span, selectors.TritYes},
		{"list neither", "a, span", b, selectors.TritNo},
		{"list maybe wins over no", "a, #home", b, selectors.TritMaybe},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sel := selectors.MustCompile(tt.selector)
			if _, got := sel.Matches(tt.node); got != tt.want {
				t.Errorf("MustCompile(%q).Match(<%s>) = %s, want %s",
					tt.selector, tt.node.TagName, got, tt.want)
			}
		})
	}
}
