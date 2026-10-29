// Ported from lib/__tests/lexer-match-type.js.

package csstree

import (
	"sync"
	"testing"
)

func TestLexerMatchType(t *testing.T) {
	singleNumber := sync.OnceValue(func() Node {
		return mustParse(t, "1", ParseOptions{Context: "value"})
	})
	severalNumbers := sync.OnceValue(func() Node {
		return mustParse(t, "1, 2, 3", ParseOptions{Context: "value"})
	})
	customSyntax := sync.OnceValue(func() *Syntax {
		return mustFork(t, ExtendFunc(func(prev *Config) *Config {
			prev.Types = map[string]string{
				"foo": "<bar>#",
				"bar": "[ 1 | 2 | 3 ]",
			}
			return prev
		}))
	})

	t.Run("should match type", func(t *testing.T) {
		match := customSyntax().Lexer.MatchType("bar", singleNumber())

		assertMatchNoError(t, `MatchType("bar", 1)`, match)
	})

	t.Run("should match type using nested", func(t *testing.T) {
		match := customSyntax().Lexer.MatchType("foo", severalNumbers())

		assertMatchNoError(t, `MatchType("foo", 1, 2, 3)`, match)
	})

	t.Run("should fail on matching wrong value", func(t *testing.T) {
		match := customSyntax().Lexer.MatchType("bar", severalNumbers())

		assertMismatch(t, `MatchType("bar", 1, 2, 3)`, match)
	})

	t.Run("should return null and save error for unknown type", func(t *testing.T) {
		match := customSyntax().Lexer.MatchType("baz", singleNumber())

		if match.Matched != nil {
			t.Errorf(`MatchType("baz", 1) matched a value, want nil`)
		}
		assertErrorMessage(t, `MatchType("baz", 1) error`, match.Error, "Unknown type `baz`")
	})

	t.Run("should not match to CSS wide keywords", func(t *testing.T) {
		for _, keyword := range testCSSWideKeywords() {
			t.Run(keyword, func(t *testing.T) {
				match := customSyntax().Lexer.MatchType("foo", mustParse(t, keyword, ParseOptions{Context: "value"}))

				if match.Matched != nil {
					t.Errorf(`MatchType("foo", %s) matched a value, want nil`, keyword)
				}
				assertErrorMessage(t, `MatchType("foo", `+keyword+`) error`, match.Error, "Mismatch\n  syntax: <bar>#\n   value: "+keyword+"\n  --------^")
			})
		}
	})
}
