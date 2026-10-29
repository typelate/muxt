package selectors

import "testing"

func TestInteractivePseudos(t *testing.T) {
	src := `<input type="checkbox">`
	for _, sel := range []string{`:checked`, `:hover`, `:focus`, `input:checked`, `:lang(en)`, `:dir(ltr)`} {
		if got := compileMatch(t, src, sel, "input"); got != TritMaybe {
			t.Errorf("%s = %v, want Maybe", sel, got)
		}
	}
}

func TestUnknownPseudoErrors(t *testing.T) {
	if _, err := Compile(`:hovr`); err == nil {
		t.Error("Compile(:hovr) = nil error, want error")
	}
}
