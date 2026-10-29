// Ported from lib/__tests/definition-syntax-parse.js.

package definitionsyntax_test

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

// assertSyntaxError mirrors assert.throws(() => parse(source), pattern):
// the error must be a *definitionsyntax.SyntaxError whose message matches pattern.
func assertSyntaxError(t *testing.T, source string, pattern *regexp.Regexp) {
	t.Helper()

	ast, err := definitionsyntax.Parse(source)
	if err == nil {
		t.Fatalf("Parse(%q) = %v, want error matching %q", source, ast, pattern)
	}

	var syntaxError *definitionsyntax.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("Parse(%q) error = %T, want *definitionsyntax.SyntaxError", source, err)
	}

	if !pattern.MatchString(err.Error()) {
		t.Errorf("Parse(%q) error = %q, want match for %q", source, err.Error(), pattern)
	}
}

func TestParse(t *testing.T) {
	t.Run("combinator precedence", func(t *testing.T) {
		combinators := []string{" ", "&&", "||", "|"} // higher goes first
		var print func(node definitionsyntax.Node) string
		print = func(node definitionsyntax.Node) string {
			switch node := node.(type) {
			case *definitionsyntax.Group:
				terms := make([]string, 0, len(node.Terms))
				for _, term := range node.Terms {
					terms = append(terms, print(term))
				}
				return "(" + strings.Join(terms, node.Combinator) + ")"
			case *definitionsyntax.Keyword:
				return node.Name
			default:
				return fmt.Sprintf("<unexpected %s>", node.Type())
			}
		}

		for i, hi := range combinators {
			for _, lo := range combinators[i+1:] {
				t.Run(`"`+hi+`" vs. "`+lo+`"`, func(t *testing.T) {
					hilo := "a" + hi + "b" + lo + "c"
					t.Run(hilo, func(t *testing.T) {
						ast, err := definitionsyntax.Parse(hilo)
						if err != nil {
							t.Fatal(err)
						}
						if got, want := print(ast), "((a"+hi+"b)"+lo+"c)"; got != want {
							t.Errorf("Parse(%q) = %s, want %s", hilo, got, want)
						}
					})

					lohi := "a" + lo + "b" + hi + "c"
					t.Run(lohi, func(t *testing.T) {
						ast, err := definitionsyntax.Parse(lohi)
						if err != nil {
							t.Fatal(err)
						}
						if got, want := print(ast), "(a"+lo+"(b"+hi+"c))"; got != want {
							t.Errorf("Parse(%q) = %s, want %s", lohi, got, want)
						}
					})
				})
			}
		}
	})

	t.Run("bad syntax", func(t *testing.T) {
		t.Run("expected a quote", func(t *testing.T) {
			assertSyntaxError(t, `'x`, regexp.MustCompile(`^Expect an apostrophe\n`))
		})

		for _, group := range []struct {
			name    string
			pattern *regexp.Regexp
			tests   []string
		}{
			{
				name:    "expected a number",
				pattern: regexp.MustCompile(`^Expect a number\n`),
				tests: []string{
					`<x>{}`,
					`<x>{ }`,
					`<x>{,2}`,
					`<x>{ ,2}`,
				},
			},
			{
				name:    "missed keyword",
				pattern: regexp.MustCompile(`^Expect a keyword\n`),
				tests: []string{
					`<>`,
					`<''>`,
				},
			},
			{
				name:    "unexpected combinator",
				pattern: regexp.MustCompile(`^Unexpected combinator\n`),
				tests: []string{
					`<x>&&`,
					`&&<x>`,
					`<x>&&||`,
				},
			},
			{
				name:    "unexpected input",
				pattern: regexp.MustCompile(`^Unexpected input\n`),
				tests: []string{
					`#`,
					`?`,
					`+`,
					`*`,
					`!`,
					`[]]`,
					`{1}`,
				},
			},
			{
				name:    "bad syntax",
				pattern: regexp.MustCompile("^Expect `.`\n"),
				tests: []string{
					`a&b`,
					`<a`,
					`[a`,
				},
			},
		} {
			t.Run(group.name, func(t *testing.T) {
				for _, test := range group.tests {
					t.Run(test, func(t *testing.T) {
						assertSyntaxError(t, test, group.pattern)
					})
				}
			})
		}
	})

	// The "parse" tests check the lexer's dictionaries; this package cannot
	// import the lexer, so they are TestDefinitionSyntaxParseLexer in
	// definition_syntax_lexer_test.go of package csstree.
}

func TestSyntaxError(t *testing.T) {
	for _, test := range []struct {
		source     string
		rawMessage string
		offset     int
		message    string
	}{
		{
			source:     `<a`,
			rawMessage: "Expect `>`",
			offset:     2,
			message:    "Expect `>`\n  <a\n----^",
		},
		{
			// offset 0 falls back to the input length, as upstream does
			source:     `&&<x>`,
			rawMessage: "Unexpected combinator",
			offset:     0,
			message:    "Unexpected combinator\n  &&<x>\n-------^",
		},
	} {
		t.Run(test.source, func(t *testing.T) {
			_, err := definitionsyntax.Parse(test.source)
			var syntaxError *definitionsyntax.SyntaxError
			if !errors.As(err, &syntaxError) {
				t.Fatalf("Parse(%q) error = %v, want *definitionsyntax.SyntaxError", test.source, err)
			}
			got := []any{syntaxError.Input, syntaxError.Offset, syntaxError.RawMessage, syntaxError.Message}
			want := []any{test.source, test.offset, test.rawMessage, test.message}
			if !slices.Equal(got, want) {
				t.Errorf("Parse(%q) error = %q, want %q", test.source, got, want)
			}
		})
	}
}
