package selectors

import "testing"

func TestNthChild(t *testing.T) {
	rows := `<table><tr id="r1"></tr><tr id="r2"></tr><tr id="r3"></tr></table>`
	for _, tt := range []struct {
		name, sel, id string
		want          Trit
	}{
		{"first", `tr:nth-child(1)`, "r1", TritYes},
		{"second not first", `tr:nth-child(1)`, "r2", TritNo},
		{"odd r3", `tr:nth-child(odd)`, "r3", TritYes},
		{"even r2", `tr:nth-child(even)`, "r2", TritYes},
		{"last-child via nth-last", `tr:nth-last-child(1)`, "r3", TritYes},
		{"all n", `tr:nth-child(n)`, "r2", TritYes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatchID(t, rows, tt.sel, tt.id); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNthChildConditional(t *testing.T) {
	// A conditional preceding sibling makes the index uncertain.
	src := `<ul>{{if .X}}<li id="a"></li>{{end}}<li id="b"></li></ul>`
	if got := compileMatchID(t, src, `li:nth-child(2)`, "b"); got != TritMaybe {
		t.Errorf("nth-child(2) on b = %v, want Maybe", got)
	}
	if got := compileMatchID(t, src, `li:nth-child(n)`, "b"); got != TritYes {
		t.Errorf("nth-child(n) on b = %v, want Yes", got)
	}
}

// compileMatchID returns the Trit for the element whose id attribute == id.
func compileMatchID(t *testing.T, src, sel, id string) Trit {
	t.Helper()
	tree := mustParse(t, src)
	s := MustCompile(sel)
	el, _ := MustCompile("[id=" + id + "]").QuerySelector(tree.Root)
	if el == nil {
		t.Fatalf("no element with id=%q in %q", id, src)
	}
	return s.trit(el)
}
