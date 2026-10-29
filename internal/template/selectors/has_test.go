package selectors

import "testing"

func TestHas(t *testing.T) {
	for _, tt := range []struct {
		name, src, sel, tag string
		want                Trit
	}{
		{"descendant yes", `<div><span class="x"></span></div>`, `div:has(.x)`, "div", TritYes},
		{"descendant no", `<div><span class="y"></span></div>`, `div:has(.x)`, "div", TritNo},
		{"child combinator", `<div><span class="x"></span></div>`, `div:has(> .x)`, "div", TritYes},
		{"conditional maybe", `<div>{{if .X}}<span class="x"></span>{{end}}</div>`, `div:has(.x)`, "div", TritMaybe},
		{"adjacent certain", `<div></div><p class="b"></p>`, `div:has(+ .b)`, "div", TritYes},
		{"adjacent no match", `<div></div><p class="b"></p>`, `div:has(+ .a)`, "div", TritNo},
		{"adjacent conditional", `<div></div>{{if .X}}<span class="a"></span>{{end}}<p class="b"></p>`, `div:has(+ .b)`, "div", TritMaybe},
		{"comma list first alt", `<div><span class="a"></span></div>`, `div:has(.a, .b)`, "div", TritYes},
		{"comma list second alt", `<div><span class="b"></span></div>`, `div:has(.a, .b)`, "div", TritYes},
		{"comma list no match", `<div><span class="c"></span></div>`, `div:has(.a, .b)`, "div", TritNo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatch(t, tt.src, tt.sel, tt.tag); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasComplexRejected(t *testing.T) {
	for _, sel := range []string{
		`div:has(.a .b)`,
		`div:has(.a > .b)`,
	} {
		if _, err := Compile(sel); err == nil {
			t.Errorf("Compile(%q) = nil error, want non-nil", sel)
		}
	}
}
