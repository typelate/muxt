// Ported from lib/__tests/lexer.js.

package csstree

import (
	"maps"
	"slices"
	"testing"
)

// testCSSWideKeywords is upstream lib/__tests/helpers cssWideKeywords.
func testCSSWideKeywords() []string {
	return []string{
		"unset",
		"initial",
		"inherit",
		"revert",
		"revert-layer",
	}
}

func mustCreateLexer(t *testing.T, config *Config) *Lexer {
	t.Helper()
	lexer, err := CreateLexer(config)
	if err != nil {
		t.Fatalf("CreateLexer() error = %v", err)
	}
	return lexer
}

func mustFork(t *testing.T, extension SyntaxExtension) *Syntax {
	t.Helper()
	syntax, err := Fork(extension)
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}
	return syntax
}

func assertMatched(t *testing.T, call string, result *MatchResult) {
	t.Helper()
	if result.Matched == nil {
		t.Errorf("%s matched = nil, want a match (error: %v)", call, result.Error)
	}
}

func assertNotMatched(t *testing.T, call string, result *MatchResult) {
	t.Helper()
	if result.Matched != nil {
		t.Errorf("%s matched a value, want nil", call)
	}
}

func TestLexer(t *testing.T) {
	t.Run("should not override generic types when used", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Generic: new(true),
			Types: map[string]string{
				"length": "foo",
			},
		})

		assertNotMatched(t, `MatchTypeString("length", "foo")`, customLexer.MatchTypeString("length", "foo"))
		assertMatched(t, `MatchTypeString("length", "1px")`, customLexer.MatchTypeString("length", "1px"))
	})

	t.Run("should not use generic type names when generics are not used", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Types: map[string]string{
				"length": "foo",
			},
		})

		assertMatched(t, `MatchTypeString("length", "foo")`, customLexer.MatchTypeString("length", "foo"))
		assertNotMatched(t, `MatchTypeString("length", "1px")`, customLexer.MatchTypeString("length", "1px"))
	})

	t.Run("validate()", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Generic: new(true),
			Types: map[string]string{
				"ref":     "<string>",
				"valid":   "<number> <ref>",
				"invalid": "<foo>",
			},
			Properties: map[string]string{
				"ref":     "<valid>",
				"valid":   "<ident> <'ref'>",
				"invalid": "<invalid>",
			},
		})

		got, err := customLexer.Validate()
		if err != nil {
			t.Fatal(err)
		}
		want := &LexerValidation{
			Errors: []string{
				"<invalid> used missed syntax definition <foo>",
				"<'invalid'> used broken syntax definition <invalid>",
			},
			Types: []string{
				"invalid",
			},
			Properties: []string{
				"invalid",
			},
		}
		if got == nil ||
			!slices.Equal(got.Errors, want.Errors) ||
			!slices.Equal(got.Types, want.Types) ||
			!slices.Equal(got.Properties, want.Properties) {
			t.Errorf("Validate() = %+v, want %+v", got, want)
		}
	})

	t.Run("should allow override units", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Generic: new(true),
			Units: map[string][]string{
				"length": {"xx", "yy"},
			},
		})

		if got := customLexer.Units()["length"]; !slices.Equal(got, []string{"xx", "yy"}) {
			t.Errorf("Units()[length] = %q, want %q", got, []string{"xx", "yy"})
		}
		assertNotMatched(t, `MatchTypeString("length", "1px")`, customLexer.MatchTypeString("length", "1px"))
		assertMatched(t, `MatchTypeString("length", "1xx")`, customLexer.MatchTypeString("length", "1xx"))
		assertMatched(t, `MatchTypeString("length", "1yy")`, customLexer.MatchTypeString("length", "1yy"))
	})

	t.Run("should not add new unit groups or discard existing", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Generic: new(true),
			Units: map[string][]string{
				"foo": {"xx", "yy"},
			},
		})

		units := customLexer.Units()
		if _, ok := units["foo"]; ok {
			t.Errorf("Units() has group foo")
		}
		if len(units) == 0 {
			t.Errorf("Units() is empty")
		}
		got := slices.Sorted(maps.Keys(units))
		want := slices.Sorted(maps.Keys(DefaultLexer().Units()))
		if !slices.Equal(got, want) {
			t.Errorf("Units() groups = %q, want %q", got, want)
		}
	})

	t.Run("should allow to override CSS wide keywords", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			CSSWideKeywords: []string{"foo", "bar"},
			Generic:         new(true),
			Properties: map[string]string{
				"test": "<number>",
			},
		})

		assertMatched(t, `MatchPropertyString("test", "foo")`, customLexer.MatchPropertyString("test", "foo"))
		assertMatched(t, `MatchPropertyString("test", "bar")`, customLexer.MatchPropertyString("test", "bar"))
		assertNotMatched(t, `MatchPropertyString("test", "inherit")`, customLexer.MatchPropertyString("test", "inherit"))
	})

	t.Run("should allow append definitions", func(t *testing.T) {
		customSyntax := mustFork(t, &Config{
			Properties: map[string]string{
				"color": "| foo",
				"new":   "| foo",
			},
			Types: map[string]string{
				"length": "| foo",
				"box":    "| foo",
				"new":    "| foo",
			},
		})

		t.Run("properties", func(t *testing.T) {
			assertMatched(t, `MatchPropertyString("color", "foo")`, customSyntax.Lexer.MatchPropertyString("color", "foo"))
			assertMatched(t, `MatchPropertyString("new", "foo")`, customSyntax.Lexer.MatchPropertyString("new", "foo"))
		})
		t.Run("types", func(t *testing.T) {
			assertMatched(t, `MatchTypeString("box", "foo")`, customSyntax.Lexer.MatchTypeString("box", "foo"))
			assertMatched(t, `MatchTypeString("new", "foo")`, customSyntax.Lexer.MatchTypeString("new", "foo"))
		})
		t.Run("should not append to generic", func(t *testing.T) {
			assertNotMatched(t, `MatchTypeString("length", "foo")`, customSyntax.Lexer.MatchTypeString("length", "foo"))
		})
	})

	t.Run("default syntax shouldn't to be broken", func(t *testing.T) {
		if got, err := DefaultLexer().Validate(); got != nil || err != nil {
			t.Errorf("DefaultLexer().Validate() = %+v, %v, want nil, nil", got, err)
		}
	})

	t.Run("dump & recovery", func(t *testing.T) {
		customLexer := mustCreateLexer(t, &Config{
			Generic:         new(true),
			CSSWideKeywords: []string{"wide"},
			Units: map[string][]string{
				"length": {"aa", "bb"},
			},
			Types: map[string]string{
				"foo": "<number> | <length>",
			},
			Properties: map[string]string{
				"test": "<foo>+",
			},
		})
		lexer := DefaultLexer()

		t.Run("custom syntax should not affect base syntax", func(t *testing.T) {
			if got, err := lexer.Validate(); got != nil || err != nil {
				t.Errorf("Validate() = %+v, %v, want nil, nil", got, err)
			}
			assertNotMatched(t, `MatchPropertyString("test", "1 2aa 3")`, lexer.MatchPropertyString("test", "1 2aa 3"))
			assertNotMatched(t, `MatchPropertyString("test", "wide")`, lexer.MatchPropertyString("test", "wide"))
			assertMatched(t, `MatchPropertyString("color", "red")`, lexer.MatchPropertyString("color", "red"))
		})

		t.Run("custom syntax should be valid and correct", func(t *testing.T) {
			if got, err := customLexer.Validate(); got != nil || err != nil {
				t.Errorf("Validate() = %+v, %v, want nil, nil", got, err)
			}
		})

		t.Run("custom syntax should match own grammar only", func(t *testing.T) {
			assertMatched(t, `MatchPropertyString("test", "1 2aa 3")`, customLexer.MatchPropertyString("test", "1 2aa 3"))
			assertMatched(t, `MatchPropertyString("test", "wide")`, customLexer.MatchPropertyString("test", "wide"))
			assertNotMatched(t, `MatchPropertyString("color", "red")`, customLexer.MatchPropertyString("color", "red"))
		})

		t.Run("recovery syntax from dump", func(t *testing.T) {
			dump, err := customLexer.Dump(false)
			if err != nil {
				t.Fatal(err)
			}
			// upstream: fork(prev => ({ ...prev, ...customLexer.dump() }))
			recoverySyntax := mustFork(t, ExtendFunc(func(prev *Config) *Config {
				prev.Generic = new(dump.Generic)
				prev.CSSWideKeywords = dump.CSSWideKeywords
				prev.Units = dump.Units
				prev.Types = dump.Types
				prev.Properties = dump.Properties
				prev.Atrules = dump.Atrules
				return prev
			}))

			if got, err := recoverySyntax.Lexer.Validate(); got != nil || err != nil {
				t.Errorf("Validate() = %+v, %v, want nil, nil", got, err)
			}
			assertMatched(t, `MatchPropertyString("test", "1 2aa 3")`, recoverySyntax.Lexer.MatchPropertyString("test", "1 2aa 3"))
			assertMatched(t, `MatchPropertyString("test", "wide")`, recoverySyntax.Lexer.MatchPropertyString("test", "wide"))
		})
	})
}
