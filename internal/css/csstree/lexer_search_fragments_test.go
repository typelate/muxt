// Ported from lib/__tests/lexer-search-fragments.js.

package csstree

import (
	"slices"
	"testing"
)

func translateFragments(t *testing.T, fragments []Fragment) []string {
	t.Helper()
	result := []string{}
	for _, fragment := range fragments {
		css, err := Generate(&Value{
			Loc:      nil,
			Children: fragment.Nodes,
		}, GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, css)
	}
	return result
}

func TestLexerSearchFragments(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("findValueFragments()", func(t *testing.T) {
		t.Run("should find single entry", func(t *testing.T) {
			declaration := mustParse(t, "border: 1px solid red", ParseOptions{Context: "declaration"}).(*Declaration)
			result, err := lexer.FindValueFragments(declaration.Property, declaration.Value, "Type", "color")
			if err != nil {
				t.Fatal(err)
			}

			if got, want := translateFragments(t, result), []string{"red"}; !slices.Equal(got, want) {
				t.Errorf("FindValueFragments() = %q, want %q", got, want)
			}
		})

		t.Run("should find multiple entries", func(t *testing.T) {
			declaration := mustParse(t, "font: 10px Arial, Courier new, Times new roman", ParseOptions{Context: "declaration"}).(*Declaration)
			result, err := lexer.FindValueFragments(declaration.Property, declaration.Value, "Type", "family-name")
			if err != nil {
				t.Fatal(err)
			}

			if got, want := translateFragments(t, result), []string{"Arial", "Courier new", "Times new roman"}; !slices.Equal(got, want) {
				t.Errorf("FindValueFragments() = %q, want %q", got, want)
			}
		})
	})

	t.Run("findDeclarationValueFragments()", func(t *testing.T) {
		t.Run("should find single entry", func(t *testing.T) {
			declaration := mustParse(t, "border: 1px solid red", ParseOptions{Context: "declaration"})
			result, err := lexer.FindDeclarationValueFragments(declaration, "Type", "color")
			if err != nil {
				t.Fatal(err)
			}

			if got, want := translateFragments(t, result), []string{"red"}; !slices.Equal(got, want) {
				t.Errorf("FindDeclarationValueFragments() = %q, want %q", got, want)
			}
		})

		t.Run("should find multiple entries", func(t *testing.T) {
			declaration := mustParse(t, "font: 10px Arial, Courier new, Times new roman", ParseOptions{Context: "declaration"})
			result, err := lexer.FindDeclarationValueFragments(declaration, "Type", "family-name")
			if err != nil {
				t.Fatal(err)
			}

			if got, want := translateFragments(t, result), []string{"Arial", "Courier new", "Times new roman"}; !slices.Equal(got, want) {
				t.Errorf("FindDeclarationValueFragments() = %q, want %q", got, want)
			}
		})
	})

	t.Run("findAllFragments()", func(t *testing.T) {
		t.Run("should find all entries in ast", func(t *testing.T) {
			ast := mustParse(t, "foo { border: 1px solid red; } bar { color: rgba(1,2,3,4); border-color: #123 rgb(1,2,3) }", ParseOptions{})
			result, err := lexer.FindAllFragments(ast, "Type", "color")
			if err != nil {
				t.Fatal(err)
			}

			if got, want := translateFragments(t, result), []string{"red", "rgba(1,2,3,4)", "#123", "rgb(1,2,3)"}; !slices.Equal(got, want) {
				t.Errorf("FindAllFragments() = %q, want %q", got, want)
			}
		})
	})
}
