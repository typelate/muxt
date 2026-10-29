package selectors

import "testing"

func TestIsWhere(t *testing.T) {
	src := `<div><h1 id="h"></h1><p id="p"></p><span id="s"></span></div>`
	for _, tt := range []struct {
		name, sel, id string
		want          Trit
	}{
		{"is match", `:is(h1, p)`, "h", TritYes},
		{"is second", `:is(h1, p)`, "p", TritYes},
		{"is no match", `:is(h1, p)`, "s", TritNo},
		{"where same as is", `:where(h1, p)`, "h", TritYes},
		{"is in compound", `div :is(p, span)`, "s", TritYes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatchID(t, src, tt.sel, tt.id); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}
