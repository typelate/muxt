// Ported from lib/__tests/lexer-match.js.

package csstree

import (
	"errors"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

func TestLexerMatch(t *testing.T) {
	customSyntax := mustFork(t, ExtendFunc(func(prev *Config) *Config {
		prev.Types = map[string]string{
			"foo": "<bar>#",
			"bar": "[ 1 | 2 | 3 ]",
			"fn":  "fn(<foo>)",
		}
		return prev
	}))

	t.Run("should match by type", func(t *testing.T) {
		value := mustParse(t, "fn(1, 2, 3)", ParseOptions{Context: "value"})
		fn := value.(*Value).Children.First()
		syntax := customSyntax.Lexer.GetType("fn")
		match := customSyntax.Lexer.MatchDescriptor(syntax, fn)

		assertMatchNoError(t, `MatchDescriptor(<fn>, fn(1, 2, 3))`, match)
	})

	t.Run("should take a string as a value", func(t *testing.T) {
		syntax := customSyntax.Lexer.GetType("fn")
		match := customSyntax.Lexer.MatchDescriptorString(syntax, "fn(1, 2, 3)")

		assertMatchNoError(t, `MatchDescriptorString(<fn>, "fn(1, 2, 3)")`, match)
	})

	t.Run("should take a string as a syntax", func(t *testing.T) {
		match := customSyntax.Lexer.MatchString("fn( <number># )", "fn(1, 2, 3)")

		assertMatchNoError(t, `MatchString("fn( <number># )", "fn(1, 2, 3)")`, match)
	})

	t.Run("should fails on bad syntax", func(t *testing.T) {
		value := mustParse(t, "fn(1, 2, 3)", ParseOptions{Context: "value"})
		fn := value.(*Value).Children.First()
		// upstream passes {}, which has no type; a nil descriptor is the Go
		// counterpart
		match := customSyntax.Lexer.MatchDescriptor(nil, fn)

		if match.Matched != nil {
			t.Errorf("MatchDescriptor(nil, fn) matched a value, want nil")
		}
		assertErrorMessage(t, "MatchDescriptor(nil, fn) error", match.Error, "Bad syntax")
	})

	t.Run("mismatch position", func(t *testing.T) {
		customSyntax := mustFork(t, ExtendFunc(func(prev *Config) *Config {
			prev.Types = map[string]string{
				"foo()": "foo( <number>#{3} )",
				"bar":   "bar( <angle> )",
				"baz()": "baz( <angle> | <number> )",
				"rgb":   "red | green | blue",
			}
			return prev
		}))

		tests := []struct {
			syntax   string
			value    string
			start    int
			end      int
			astStart int // -1 is undefined
			astEnd   int // -1 is undefined
		}{
			{syntax: "<foo()>", value: "foo(1, 2px, 3)", start: 7, end: 10, astStart: -1, astEnd: -1},
			{syntax: "<foo()>", value: "foo(1, 2, 3, 4)", start: 11, end: 12, astStart: -1, astEnd: -1},
			{syntax: "<foo()>", value: "foo(1, 211px)", start: 7, end: 12, astStart: -1, astEnd: -1},
			{syntax: "<foo()>", value: "foo(1, 2 3)", start: 9, end: 10, astStart: -1, astEnd: -1},
			{syntax: "<foo()>", value: "foo(1, 2)", start: 8, end: 9, astStart: 9, astEnd: -1}, // in this case AST match can't answer with correct location
			{syntax: "<bar>", value: "bar( foo )", start: 5, end: 8, astStart: -1, astEnd: -1},
			{syntax: "<baz()>", value: "baz( foo )", start: 5, end: 8, astStart: -1, astEnd: -1},
			{syntax: "<baz()>", value: "baz( 1px )", start: 5, end: 8, astStart: -1, astEnd: -1},
			{syntax: "<number>{4}", value: "1 2 3", start: 5, end: 5, astStart: -1, astEnd: -1},
			{syntax: "<number>#{4}", value: "1, 2, 3", start: 7, end: 7, astStart: -1, astEnd: -1},
			{syntax: "<number>#{4}", value: "1, 2, 3,", start: 8, end: 8, astStart: -1, astEnd: -1},
			{syntax: "<number>#{4}", value: "1, 2, 3, 4,", start: 10, end: 11, astStart: -1, astEnd: -1},
			{syntax: "<rgb>+", value: "yellow", start: 0, end: 6, astStart: -1, astEnd: -1},
			{syntax: "<rgb>+", value: "yellow red", start: 0, end: 6, astStart: -1, astEnd: -1},
			{syntax: "<rgb>+", value: "red yellow", start: 4, end: 10, astStart: -1, astEnd: -1},
			{syntax: "<rgb>+", value: "red yellow green", start: 4, end: 10, astStart: -1, astEnd: -1},
		}

		generate := func(t *testing.T, ast Node) string {
			css, err := Generate(ast, GenerateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			return css
		}

		values := []struct {
			typ       string
			match     func(t *testing.T, syntax, value string) *MatchResult
			serialize func(t *testing.T, value string) string
		}{
			{
				"string",
				func(t *testing.T, syntax, value string) *MatchResult {
					return customSyntax.Lexer.MatchString(syntax, value)
				},
				func(t *testing.T, value string) string { return value },
			},
			{
				"ast",
				func(t *testing.T, syntax, value string) *MatchResult {
					return customSyntax.Lexer.Match(syntax, mustParse(t, value, ParseOptions{Context: "value", Positions: true}))
				},
				func(t *testing.T, value string) string {
					return generate(t, mustParse(t, value, ParseOptions{Context: "value", Positions: true}))
				},
			},
			{
				"ast Raw",
				func(t *testing.T, syntax, value string) *MatchResult {
					return customSyntax.Lexer.Match(syntax, rawTestValue(value))
				},
				func(t *testing.T, value string) string { return generate(t, rawTestValue(value)) },
			},
		}

		for _, v := range values {
			t.Run(v.typ+" value", func(t *testing.T) {
				for _, test := range tests {
					t.Run(test.syntax+" -> "+test.value, func(t *testing.T) {
						match := v.match(t, test.syntax, test.value)

						if match.Matched != nil {
							t.Errorf("Match(%q, %q) matched a value, want nil", test.syntax, test.value)
						}
						var matchError *SyntaxMatchError
						if !errors.As(match.Error, &matchError) {
							t.Fatalf("Match(%q, %q) error = %v, want a *SyntaxMatchError", test.syntax, test.value, match.Error)
						}
						if matchError.Offset != matchError.Loc.Start.Offset {
							t.Errorf("Match(%q, %q) error.Offset = %d, want loc.start.offset %d", test.syntax, test.value, matchError.Offset, matchError.Loc.Start.Offset)
						}
						wantStart, wantEnd := test.start, test.end
						if v.typ == "ast" && test.astStart != -1 {
							wantStart = test.astStart
						}
						if v.typ == "ast" && test.astEnd != -1 {
							wantEnd = test.astEnd
						}
						if matchError.Loc.Start.Offset != wantStart {
							t.Errorf("Match(%q, %q) loc.start = %d, want %d", test.syntax, test.value, matchError.Loc.Start.Offset, wantStart)
						}
						if matchError.Loc.End.Offset != wantEnd {
							t.Errorf("Match(%q, %q) loc.end = %d, want %d", test.syntax, test.value, matchError.Loc.End.Offset, wantEnd)
						}
						wantMessage := "Mismatch\n  syntax: " + test.syntax +
							"\n   value: " + v.serialize(t, test.value) +
							"\n  --------" + strings.Repeat("-", matchError.MismatchOffset) + "^"
						if matchError.Message != wantMessage {
							t.Errorf("Match(%q, %q) message = %q, want %q", test.syntax, test.value, matchError.Message, wantMessage)
						}
					})
				}
			})
		}
	})
}

// rawTestValue is upstream's { type: 'Raw', value, loc } test value.
func rawTestValue(value string) Node {
	return &Raw{
		Value: value,
		Loc: &Location{
			Start: tokenizer.Position{Line: 1, Column: 1, Offset: 0},
			End:   tokenizer.Position{Line: 1, Column: len(value) + 1, Offset: len(value)},
		},
	}
}
