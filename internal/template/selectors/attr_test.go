package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

func TestSelectorMatch_IDClassAttribute(t *testing.T) {
	// a: fully static. b: dynamic id/class values, plus a static bare attr.
	// f: a class token glued to a dynamic part (boundary case).
	tree := parseHTML(t, `<a id="home" class="nav active" href="/"></a>`+
		`<b id="{{.ID}}" class="card {{.Extra}}" data-x></b>`+
		`<f class="ca{{.X}}rd nav"></f>`)
	a := firstElement(t, tree, "a")
	b := firstElement(t, tree, "b")
	f := firstElement(t, tree, "f")

	for _, tt := range []struct {
		name     string
		selector string
		node     *template.HTMLElementNode
		want     selectors.Trit
	}{
		// id: exact value
		{"id static hit", "#home", a, selectors.TritYes},
		{"id static miss", "#nope", a, selectors.TritNo},
		{"id dynamic value", "#home", b, selectors.TritMaybe},

		// class: word membership
		{"class static hit", ".nav", a, selectors.TritYes},
		{"class static second token", ".active", a, selectors.TritYes},
		{"class static miss", ".missing", a, selectors.TritNo},
		{"class static token beside dynamic", ".card", b, selectors.TritYes},
		{"class dynamic token", ".other", b, selectors.TritMaybe},

		// class boundary: token glued to a dynamic part is not a certain match
		{"class whitespace-bounded token still certain", ".nav", f, selectors.TritYes},
		{"class token glued to dynamic is uncertain", ".card", f, selectors.TritMaybe},
		{"class partial glued token", ".ca", f, selectors.TritMaybe},

		// [attr] presence
		{"attr present", "[href]", a, selectors.TritYes},
		{"attr absent", "[data-x]", a, selectors.TritNo},
		{"bare attr present", "[data-x]", b, selectors.TritYes},

		// [attr=value] exact
		{"attr value hit", `[href="/"]`, a, selectors.TritYes},
		{"attr value miss", `[href="/other"]`, a, selectors.TritNo},
		{"attr value dynamic", `[id="home"]`, b, selectors.TritMaybe},
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
