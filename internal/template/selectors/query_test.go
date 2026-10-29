package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

func TestQuery(t *testing.T) {
	tree := parseHTML(t, `<main><section class="card"><h2>A</h2><p id="x">x</p></section>`+
		`{{if .X}}<section class="card"><h2>B</h2></section>{{end}}</main>`+
		`{{define "d"}}<aside class="card"></aside>{{end}}`)
	root := tree.Root

	t.Run("QuerySelectorAll yields matches with their Trit, in document order", func(t *testing.T) {
		var elems []*template.HTMLElementNode
		var trits []selectors.Trit
		for el, tr := range selectors.MustCompile(".card").QuerySelectorAll(root) {
			elems = append(elems, el)
			trits = append(trits, tr)
		}
		// Two .card sections; the second is inside {{if}} so it is Maybe. The
		// .card aside lives in a detached {{define}} and is not traversed.
		if len(elems) != 2 {
			t.Fatalf("QuerySelectorAll(.card) yielded %d elements, want 2", len(elems))
		}
		if trits[0] != selectors.TritYes {
			t.Errorf("trit[0] = %s, want TritYes", trits[0])
		}
		if trits[1] != selectors.TritMaybe {
			t.Errorf("trit[1] = %s, want TritMaybe", trits[1])
		}
	})

	t.Run("QuerySelector returns the first match in document order", func(t *testing.T) {
		el, tr := selectors.MustCompile("h2").QuerySelector(root)
		if el == nil {
			t.Fatal("QuerySelector(h2) returned nil, want first h2")
		}
		if el.TagName != "h2" || tr != selectors.TritYes {
			t.Errorf("QuerySelector(h2) = <%s> %s, want <h2> TritYes", el.TagName, tr)
		}
	})

	t.Run("QuerySelector reports no match", func(t *testing.T) {
		if el, tr := selectors.MustCompile(".missing").QuerySelector(root); el != nil || tr != selectors.TritNo {
			t.Errorf("QuerySelector(.missing) = %v %s, want nil TritNo", el, tr)
		}
	})

	t.Run("Closest walks up from the start node", func(t *testing.T) {
		p := elementByID(t, tree, "x")
		if el, tr := selectors.MustCompile("section.card").Closest(p); el == nil || el.TagName != "section" || tr != selectors.TritYes {
			t.Errorf("Closest(section.card) from #x = %v %s, want a TritYes <section>", el, tr)
		}
		if el, _ := selectors.MustCompile("main").Closest(p); el == nil || el.TagName != "main" {
			t.Errorf("Closest(main) from #x = %v, want <main>", el)
		}
		if el, _ := selectors.MustCompile(".nope").Closest(p); el != nil {
			t.Errorf("Closest(.nope) = %v, want nil", el)
		}
	})
}
