// Ported from lib/__tests/definition-syntax-generate.js.

package definitionsyntax_test

import (
	"regexp"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

type parseGenerateTest struct {
	source   string
	generate string
}

func runParseGenerateTest(t *testing.T, test parseGenerateTest) {
	t.Helper()

	name := test.source
	if test.generate != "" {
		name += " → " + test.generate
	}

	t.Run(name, func(t *testing.T) {
		ast, err := definitionsyntax.Parse(test.source)
		if err != nil {
			t.Fatal(err)
		}
		actual := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{})

		want := test.generate
		if want == "" {
			want = test.source
		}
		if actual != want {
			t.Errorf("Generate(Parse(%q)) = %q, want %q", test.source, actual, want)
		}
	})
}

func TestGenerate(t *testing.T) {
	t.Run("should throw an exception on bad node type", func(t *testing.T) {
		t.Skip("not ported: Node is a sealed interface, so a node of an unknown type cannot be constructed")
	})

	t.Run("round trip", func(t *testing.T) {
		t.Run("multiplers", func(t *testing.T) {
			for _, test := range []parseGenerateTest{
				{source: `<number>`},
				{source: `<number>*`},
				{source: `<number>?`},
				{source: `<number>+`},
				{source: `<number>#`},
				{source: `<number>#{2,3}`},
				{source: `<number>{2}`},
				{source: `<number>{2,}`},
				{source: `<number>{2,3}`},
				{source: `<number>{0,}`, generate: `<number>*`},
				{source: `<number>{1,}`, generate: `<number>+`},
				{source: `<number>{2,2}`, generate: `<number>{2}`},
				{source: `[ <number> ]`},
				{source: `[ <number> ]?`},
				{source: `[ <number> ]*`},
				{source: `[ <number> ]+`},
				{source: `[ <number> ]#`},
				{source: `[ <number> ]#{1,2}`},
				{source: `[ <number> ]!`},
				{source: `[ <number> ]!{1,2}`},
				{source: `[ <number> ]!#{1,2}`},
			} {
				runParseGenerateTest(t, test)
			}
		})

		t.Run("bracketed range notation", func(t *testing.T) {
			for _, test := range []parseGenerateTest{
				{source: `<number>`},
				{source: `<number [1,2]>`},
				{source: `<number [-∞,2]>`},
				{source: `<number [1,∞]>`},
				{source: `<number [-∞,∞]>`, generate: `<number [-∞,∞]>`},
				{source: `<number[1,2]>`, generate: `<number [1,2]>`},
				{source: `<number [1 , 2]>`, generate: `<number [1,2]>`},
			} {
				runParseGenerateTest(t, test)
			}
		})

		t.Run("fixture", func(t *testing.T) {
			spacesBeforeComma := regexp.MustCompile(`\s+,`)
			spacesAfterLeftBrace := regexp.MustCompile(`\{\s+(\d)`)
			spacesBeforeRightBrace := regexp.MustCompile(`(\d)\s+\}`)
			spacesAfterCommaInRange := regexp.MustCompile(`,\s+(\d+)\}`)

			for _, testFile := range loadDefinitionSyntaxTests(t) {
				for _, test := range testFile.Tests {
					if test.Syntax == "" {
						continue
					}

					t.Run(test.Name, func(t *testing.T) {
						ast, err := definitionsyntax.Parse(test.Syntax)
						if err != nil {
							t.Fatal(err)
						}
						actual := spacesBeforeComma.ReplaceAllString(definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{}), ",")
						expected := spacesBeforeComma.ReplaceAllString(test.Syntax, ",")
						expected = spacesAfterLeftBrace.ReplaceAllString(expected, "{$1")
						expected = spacesBeforeRightBrace.ReplaceAllString(expected, "$1}")
						expected = spacesAfterCommaInRange.ReplaceAllString(expected, ",$1}")

						if actual != expected {
							t.Errorf("Generate(Parse(%q)) = %q, want %q", test.Syntax, actual, expected)
						}
					})
				}
			}
		})

		// "lexer dictionary" checks the lexer's dictionaries; this package
		// cannot import the lexer, so it is TestDefinitionSyntaxGenerateLexer
		// in definition_syntax_lexer_test.go of package csstree.
	})

	t.Run("using forceBraces", func(t *testing.T) {
		const source = `a b   |   c ||   d &&   e f`
		ast, err := definitionsyntax.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		actual := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{ForceBraces: true})
		const expected = `[ [ a b ] | [ c || [ d && [ e f ] ] ] ]`

		if actual != expected {
			t.Errorf("Generate(Parse(%q), {forceBraces}) = %q, want %q", source, actual, expected)
		}
	})

	t.Run("using decorate", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			const source = `<foo> && <bar>`
			ast, err := definitionsyntax.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			const expected = `*{<foo>} && {<bar>}*`
			actual := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{
				Decorate: func(str string, node definitionsyntax.Node) string {
					switch node.Type() {
					case "Type":
						return "{" + str + "}"

					case "Group":
						return "*" + str + "*"

					default:
						return str
					}
				},
			})

			if actual != expected {
				t.Errorf("Generate(Parse(%q), decorate) = %q, want %q", source, actual, expected)
			}
		})

		t.Run("all the node types", func(t *testing.T) {
			const source = `<foo> && <'bar'> | [ ( a+, b( 'c' / <d>#{1,2} ) ) ]!`
			ast, err := definitionsyntax.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			const expected = `{Group}{Group}{Type}<foo>{/Type} && {Property}<'bar'>{/Property}{/Group} | {Group}[ {Token}({/Token} {Keyword}a{/Keyword}{Multiplier}+{/Multiplier} {Comma},{/Comma} {Function}b({/Function} {String}'c'{/String} {Token}/{/Token} {Type}<d>{/Type}{Multiplier}#{1,2}{/Multiplier} {Token}){/Token} {Token}){/Token} ]!{/Group}{/Group}`
			actual := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{
				Decorate: func(str string, node definitionsyntax.Node) string {
					return "{" + node.Type() + "}" + str + "{/" + node.Type() + "}"
				},
			})

			if actual != expected {
				t.Errorf("Generate(Parse(%q), decorate) = %q, want %q", source, actual, expected)
			}
		})
	})
}
