package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestSpecificity(t *testing.T) {
	for _, test := range []struct {
		selector string
		want     Specificity
	}{
		{`*`, Specificity{0, 0, 0}},
		{`li`, Specificity{0, 0, 1}},
		{`ul li`, Specificity{0, 0, 2}},
		{`.a`, Specificity{0, 1, 0}},
		{`#x`, Specificity{1, 0, 0}},
		{`a[href]`, Specificity{0, 1, 1}},
		{`a:hover`, Specificity{0, 1, 1}},
		{`p::before`, Specificity{0, 0, 2}},
		{`p:before`, Specificity{0, 0, 2}},
		{`:is(#a, .b) p`, Specificity{1, 0, 1}},
		{`:where(#a) p`, Specificity{0, 0, 1}},
		{`:not(.a, #b)`, Specificity{1, 0, 0}},
		{`li:nth-child(2n of .x)`, Specificity{0, 2, 1}},
		{`li:nth-of-type(2n)`, Specificity{0, 1, 1}},
		{`.a:has(> img)`, Specificity{0, 1, 1}},
		{`& .b`, Specificity{0, 2, 0}}, // nested in a rule whose selector has specificity (0, 1, 0)
	} {
		t.Run(test.selector, func(t *testing.T) {
			ast, err := csstree.Parse(test.selector, csstree.ParseOptions{Context: "selector"})
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", test.selector, err)
			}
			sel, ok := ast.(*csstree.Selector)
			if !ok {
				t.Fatalf("Parse(%q) = %T, want *csstree.Selector", test.selector, ast)
			}

			got := selectorSpecificity(sel, Specificity{0, 1, 0})

			if got != test.want {
				t.Errorf("selectorSpecificity(%q) = %s, want %s", test.selector, got, test.want)
			}
		})
	}
}

func TestSelectorAt(t *testing.T) {
	const page = `<style>.a { color: red } p.b {} .a:has(.b .c) {}</style>` +
		`<p class="a"></p>{{if .X}}<p class="a b"></p>{{end}}`
	for _, test := range []struct {
		name, input, cursor string
		found               bool
		want                SelectorInfo // Pos and End are checked against the cursor
	}{
		{
			name: "class selector", input: page, cursor: ".a {", found: true,
			want: SelectorInfo{Text: ".a", Specificity: Specificity{0, 1, 0}, Matches: 1, ConditionalMatches: 1},
		},
		{
			name: "compound selector", input: page, cursor: "p.b", found: true,
			want: SelectorInfo{Text: "p.b", Specificity: Specificity{0, 1, 1}, ConditionalMatches: 1},
		},
		{
			name: "selector htt cannot match", input: page, cursor: ".a:has", found: true,
			want: SelectorInfo{Text: ".a:has(.b .c)", Specificity: Specificity{0, 3, 0}, Unchecked: true},
		},
		{
			name: "nested selector", input: `<style>.a { & > .b {} }</style><div class="a"><p class="b"></p></div>`, cursor: "& > .b", found: true,
			want: SelectorInfo{Text: "& > .b", Specificity: Specificity{0, 2, 0}, Matches: 1},
		},
		{name: "declaration", input: page, cursor: "color"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			pos := lex.Pos(strings.Index(test.input, test.cursor))

			got, ok := SelectorAt(tree, pos)

			if ok != test.found {
				t.Fatalf("SelectorAt(%q) found = %t, want %t", test.cursor, ok, test.found)
			}
			if !ok {
				return
			}
			if got.Pos > pos || got.End <= pos {
				t.Errorf("selector span [%d, %d) does not contain the cursor %d", got.Pos, got.End, pos)
			}
			got.Pos, got.End = 0, 0
			if got != test.want {
				t.Errorf("SelectorAt(%q) = %+v, want %+v", test.cursor, got, test.want)
			}
		})
	}
}

func TestStyleDeclarationAtInStyleElement(t *testing.T) {
	const input = `<style>.a { color: red; margin: 0 }</style>`
	tree, err := template.ParseTree("test.gohtml", "test", input, nil, lex.NewOptions())
	if err != nil {
		t.Fatalf("ParseTree error: %v", err)
	}

	t.Run("declaration", func(t *testing.T) {
		got, ok := StyleDeclarationAt(tree, lex.Pos(strings.Index(input, "margin")))
		if !ok || got.Property != "margin" || got.Value != "0" || !got.Shorthand {
			t.Errorf("StyleDeclarationAt(margin) = %+v, %t; want the margin shorthand with value 0", got, ok)
		}
	})
	t.Run("selector", func(t *testing.T) {
		if got, ok := StyleDeclarationAt(tree, lex.Pos(strings.Index(input, ".a"))); ok {
			t.Errorf("StyleDeclarationAt(.a) = %+v, want none", got)
		}
	})
}
