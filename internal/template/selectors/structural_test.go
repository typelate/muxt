package selectors

import "testing"

func TestRootAndOnlyChild(t *testing.T) {
	for _, tt := range []struct {
		name, src, sel, tag string
		want                Trit
	}{
		{"root top-level", `<html></html>`, `:root`, "html", TritYes},
		{"root nested no", `<html><body></body></html>`, `:root`, "body", TritNo},
		{"only-child yes", `<ul><li></li></ul>`, `:only-child`, "li", TritYes},
		{"only-child no", `<ul><li></li><li></li></ul>`, `:only-child`, "li", TritNo},
		{"only-child maybe", `<ul><li></li>{{if .X}}<li></li>{{end}}</ul>`, `:only-child`, "li", TritMaybe},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatch(t, tt.src, tt.sel, tt.tag); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}
