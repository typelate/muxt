package selectors

import (
	"strings"
	"testing"
)

func TestIdentifierEscapes(t *testing.T) {
	// A class literally named "a.b" is written .a\.b in a selector.
	src := `<div class="a.b"></div>`
	if got := compileMatch(t, src, `.a\.b`, "div"); got != TritYes {
		t.Errorf("escaped class = %v, want Yes", got)
	}
}

func TestEscapedAttributeValues(t *testing.T) {
	// Unquoted attribute values must honor escapes: [data-x=a\.b] matches data-x="a.b"
	src := `<div data-x="a.b"></div>`
	if got := compileMatch(t, src, `[data-x=a\.b]`, "div"); got != TritYes {
		t.Errorf("escaped unquoted attr value = %v, want Yes", got)
	}

	// Normal unquoted values still work: [href$=.png] matches href="/a.png"
	src = `<a href="/a.png"></a>`
	if got := compileMatch(t, src, `[href$=.png]`, "a"); got != TritYes {
		t.Errorf("normal unquoted attr value = %v, want Yes", got)
	}
}

func TestParserDoesNotHangOnGarbage(t *testing.T) {
	// Adversarial inputs must return an error, not loop forever.
	for _, in := range []string{`:has(`, `[a`, `:is(a`, strings.Repeat("(", 500), `\`} {
		if _, err := Compile(in); err == nil {
			t.Errorf("Compile(%q) = nil error, want error", in)
		}
	}
}

func FuzzCompile(f *testing.F) {
	for _, s := range []string{`a`, `.x`, `#y`, `[a=b]`, `:has(> .z)`, `a > b + c ~ d`, `:nth-child(2n+1)`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// Must not panic or hang; an error is fine.
		_, _ = Compile(s)
	})
}
