package selectors

import "testing"

// TestAttributeValueParsing pins the boundaries of the attribute-value reader
// (parser.readValue): bare vs quoted vs escaped values, and the error edges
// (unterminated string, empty bare value). Attribute-value selectors like
// [name="x"] are used directly by the form-analysis feature, so their parsing
// boundaries are worth asserting tightly (mutation testing left readValue's
// boundary conditions alive under the looser existing tests).
func TestAttributeValueParsing(t *testing.T) {
	for _, tt := range []struct {
		name, src, sel string
		want           Trit
	}{
		{"bare value terminated by ]", `<a name="foo"></a>`, `[name=foo]`, TritYes},
		{"bare value mismatch", `<a name="foo"></a>`, `[name=bar]`, TritNo},
		{"double-quoted with space", `<a name="foo bar"></a>`, `[name="foo bar"]`, TritYes},
		{"single-quoted with space", `<a name="foo bar"></a>`, `[name='foo bar']`, TritYes},
		{"quoted exact vs bare-would-split", `<a name="foo bar"></a>`, `[name="foo"]`, TritNo},
		{"escaped closing bracket in value", `<a name="a]b"></a>`, `[name="a]b"]`, TritYes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatch(t, tt.src, tt.sel, "a"); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAttributeValueParseErrors pins the readValue error boundaries: an
// unterminated quoted string and an empty bare value must fail to compile.
func TestAttributeValueParseErrors(t *testing.T) {
	for _, sel := range []string{
		`[name="unterminated`,
		`[name=]`,
	} {
		if _, err := Compile(sel); err == nil {
			t.Errorf("Compile(%q) = nil error, want a parse error", sel)
		}
	}
}
