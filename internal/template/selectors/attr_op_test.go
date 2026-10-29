package selectors

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// mustParse parses src and returns the tree, or fatals if it can't.
func mustParse(t *testing.T, src string) *template.Tree {
	t.Helper()
	tree, err := template.ParseTree("t", "t", src, nil, lex.Options{ParseHTML: true})
	if err != nil {
		t.Fatalf("ParseTree(%q) error: %v", src, err)
	}
	return tree
}

// compileMatch parses src, compiles sel, and returns the Trit for the first
// element named tag in document order.
func compileMatch(t *testing.T, src, sel, tag string) Trit {
	t.Helper()
	tree := mustParse(t, src)
	s, err := Compile(sel)
	if err != nil {
		t.Fatalf("Compile(%q) error: %v", sel, err)
	}
	el, _ := MustCompile(tag).QuerySelector(tree.Root)
	if el == nil {
		t.Fatalf("no <%s> found in %q", tag, src)
	}
	return s.trit(el)
}

func TestAttributeOperators(t *testing.T) {
	for _, tt := range []struct {
		name, src, sel string
		want           Trit
	}{
		{"includes yes", `<a class="x y z"></a>`, `[class~=y]`, TritYes},
		{"includes no", `<a class="xy"></a>`, `[class~=y]`, TritNo},
		{"dash exact", `<a lang="en"></a>`, `[lang|=en]`, TritYes},
		{"dash prefix", `<a lang="en-US"></a>`, `[lang|=en]`, TritYes},
		{"dash no", `<a lang="fr"></a>`, `[lang|=en]`, TritNo},
		{"prefix yes", `<a href="https://x"></a>`, `[href^=https]`, TritYes},
		{"prefix no", `<a href="http://x"></a>`, `[href^=https]`, TritNo},
		{"suffix yes", `<a href="/a.png"></a>`, `[href$=.png]`, TritYes},
		{"substring yes", `<a href="/a/b/c"></a>`, `[href*=/b/]`, TritYes},
		{"case-insensitive", `<a type="SUBMIT"></a>`, `[type=submit i]`, TritYes},
		{"case-sensitive default", `<a type="SUBMIT"></a>`, `[type=submit]`, TritNo},
		{"dynamic value maybe", `<a class="{{.C}}"></a>`, `[class~=y]`, TritMaybe},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := compileMatch(t, tt.src, tt.sel, "a"); got != tt.want {
				t.Errorf("trit = %v, want %v", got, tt.want)
			}
		})
	}
}
