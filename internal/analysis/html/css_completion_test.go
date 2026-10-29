package analysis

import (
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCSSCompletionAt(t *testing.T) {
	for _, test := range []struct {
		name, input, before string // the cursor is right after before
		found               bool
		property, partial   string
		includes            []string
	}{
		{
			name: "property name in a style attribute", input: `<p style="col"></p>`, before: `col`,
			found: true, partial: "col", includes: []string{"color", "column-count"},
		},
		{
			name: "property name after a declaration", input: `<p style="color: red; marg"></p>`, before: `marg`,
			found: true, partial: "marg", includes: []string{"margin", "margin-top"},
		},
		{
			name: "value keyword from the property syntax", input: `<p style="color: re"></p>`, before: `re`,
			found: true, property: "color", partial: "re", includes: []string{"red", "currentcolor", "inherit"},
		},
		{
			name: "empty value", input: `<p style="display: "></p>`, before: `display: `,
			found: true, property: "display", includes: []string{"flex", "grid", "none"},
		},
		{
			name: "property name in a style rule", input: `<style>.a { disp }</style>`, before: `disp`,
			found: true, partial: "disp", includes: []string{"display"},
		},
		{
			name: "value in a style rule", input: `<style>.a { display: fl }</style>`, before: `fl`,
			found: true, property: "display", partial: "fl", includes: []string{"flex"},
		},
		{name: "selector", input: `<style>.a {}</style>`, before: `.a`},
		{name: "other attribute", input: `<p title="col"></p>`, before: `col`},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			pos := lex.Pos(strings.Index(test.input, test.before) + len(test.before))

			got, ok := CSSCompletionAt(tree, pos)

			if ok != test.found {
				t.Fatalf("CSSCompletionAt(%q) found = %t, want %t (%+v)", test.before, ok, test.found, got)
			}
			if !ok {
				return
			}
			if got.Property != test.property || got.Partial != test.partial {
				t.Errorf("CSSCompletionAt(%q) property, partial = %q, %q; want %q, %q", test.before, got.Property, got.Partial, test.property, test.partial)
			}
			if want := pos - lex.Pos(len(test.partial)); got.Start != want {
				t.Errorf("Start = %d, want %d", got.Start, want)
			}
			for _, name := range test.includes {
				if !slices.Contains(got.Candidates, name) {
					t.Errorf("candidates do not include %q", name)
				}
			}
		})
	}
}
