package selectors

import "testing"

func TestOfType(t *testing.T) {
	src := `<div><h2 id="h1"></h2><p id="p1"></p><h2 id="h2"></h2></div>`
	for _, tt := range []struct {
		name, sel, id string
		want          Trit
	}{
		{"first-of-type h1", `h2:first-of-type`, "h1", TritYes},
		{"first-of-type h2 no", `h2:first-of-type`, "h2", TritNo},
		{"last-of-type h2", `h2:last-of-type`, "h2", TritYes},
		{"only-of-type p", `p:only-of-type`, "p1", TritYes},
		{"only-of-type h2 no", `h2:only-of-type`, "h1", TritNo},
		{"nth-of-type 2", `h2:nth-of-type(2)`, "h2", TritYes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatchID(t, src, tt.sel, tt.id); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}
