// Ported from lib/__tests/lexer-match-atrule-descriptor.js and
// lib/__tests/lexer-match-atrule-prelude.js.

package csstree

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestLexerMatchAtruleDescriptor(t *testing.T) {
	lexer := DefaultLexer()
	swapValue := sync.OnceValue(func() Node {
		return mustParse(t, "swap", ParseOptions{Context: "value"})
	})
	xxxValue := sync.OnceValue(func() Node {
		return mustParse(t, "xxx", ParseOptions{Context: "value"})
	})
	const fontDisplaySyntax = "auto | block | swap | fallback | optional"
	customSyntax := sync.OnceValue(func() *Syntax {
		return mustFork(t, ExtendFunc(func(prev *Config) *Config {
			prev.Atrules = map[string]AtruleDefinition{
				"font-face": {
					Descriptors: map[string]string{
						"font-display":      fontDisplaySyntax,
						"-foo-font-display": fontDisplaySyntax + " | xxx",
					},
				},
			}
			return prev
		}))
	})

	t.Run("should match", func(t *testing.T) {
		match := customSyntax().Lexer.MatchAtruleDescriptor("font-face", "font-display", swapValue())

		assertMatchNoError(t, `MatchAtruleDescriptor("font-face", "font-display", swap)`, match)
	})

	t.Run("vendor prefixes", func(t *testing.T) {
		t.Run("vendor prefix in keyword name", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtruleDescriptor("-prefix-font-face", "font-display", swapValue())

			assertMatchNoError(t, `MatchAtruleDescriptor("-prefix-font-face", "font-display", swap)`, match)
		})

		t.Run("vendor prefix in declarator name", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtruleDescriptor("font-face", "-prefix-font-display", swapValue())

			assertMatchNoError(t, `MatchAtruleDescriptor("font-face", "-prefix-font-display", swap)`, match)
		})

		t.Run("case insensetive with vendor prefix", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtruleDescriptor("FONT-FACE", "FONT-DISPLAY", swapValue())
			assertMatchNoError(t, `MatchAtruleDescriptor("FONT-FACE", "FONT-DISPLAY", swap)`, match)

			match = customSyntax().Lexer.MatchAtruleDescriptor("FONT-face", "-VENDOR-Font-Display", swapValue())
			assertMatchNoError(t, `MatchAtruleDescriptor("FONT-face", "-VENDOR-Font-Display", swap)`, match)
		})

		t.Run("should use verdor version first", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtruleDescriptor("font-face", "-foo-font-display", xxxValue())
			assertMatchNoError(t, `MatchAtruleDescriptor("font-face", "-foo-font-display", xxx)`, match)

			match = customSyntax().Lexer.MatchAtruleDescriptor("font-face", "font-display", xxxValue())
			if match.Matched != nil {
				t.Errorf(`MatchAtruleDescriptor("font-face", "font-display", xxx) matched a value, want nil`)
			}
			assertErrorMessage(t, `MatchAtruleDescriptor("font-face", "font-display", xxx) error`, match.Error, "Mismatch\n  syntax: "+fontDisplaySyntax+"\n   value: xxx\n  --------^")
		})
	})

	t.Run("should not be matched to empty value", func(t *testing.T) {
		match := lexer.MatchAtruleDescriptor("font-face", "font-display", mustParse(t, "", ParseOptions{Context: "value", Positions: true}))

		assertNotMatchedAt(t, `MatchAtruleDescriptor("font-face", "font-display", "")`, match, 1, 1)
	})

	t.Run("should not be matched to at-rules with no descriptors", func(t *testing.T) {
		match := lexer.MatchAtruleDescriptor("keyframes", "font-face", swapValue())

		if match.Matched != nil {
			t.Errorf(`MatchAtruleDescriptor("keyframes", "font-face", swap) matched a value, want nil`)
		}
		assertErrorMessage(t, `MatchAtruleDescriptor("keyframes", "font-face", swap) error`, match.Error, "At-rule `@keyframes` has no known descriptors")
	})

	t.Run("should not match css wide keywords", func(t *testing.T) {
		for _, keyword := range testCSSWideKeywords() {
			t.Run(keyword, func(t *testing.T) {
				match := lexer.MatchAtruleDescriptor("font-face", "font-display", mustParse(t, keyword, ParseOptions{Context: "value"}))

				assertMismatch(t, `MatchAtruleDescriptor("font-face", "font-display", `+keyword+`)`, match)
			})
		}
	})

	forEachAtruleDescriptorTest(t, func(t *testing.T, testType, testState, name string, lexer *Lexer, atruleName, descriptorName, value string) {
		switch testType {
		case "valid":
			runDefinitionSyntaxValue(t, testState, name, func(t *testing.T) {
				match := lexer.MatchAtruleDescriptorString(atruleName, descriptorName, value)

				if match.Matched == nil {
					t.Errorf("MatchAtruleDescriptorString(%q, %q, %q) = nil, want a match: %v", atruleName, descriptorName, value, match.Error)
				}
			})

		case "invalid":
			runDefinitionSyntaxValue(t, testState, name, func(t *testing.T) {
				match := lexer.MatchAtruleDescriptorString(atruleName, descriptorName, value)

				assertSyntaxMatchError(t, `MatchAtruleDescriptorString("`+atruleName+`", "`+descriptorName+`", "`+value+`")`, match)
			})
		}
	})
}

// assertMismatch checks a "Mismatch" error (upstream
// match.error.rawMessage === 'Mismatch').
func assertMismatch(t *testing.T, call string, match *MatchResult) {
	t.Helper()
	if match.Matched != nil {
		t.Errorf("%s matched a value, want nil", call)
	}
	var matchError *SyntaxMatchError
	if !errors.As(match.Error, &matchError) || matchError.RawMessage != "Mismatch" {
		t.Errorf("%s error = %v, want a Mismatch", call, match.Error)
	}
}

func TestLexerMatchAtrulePrelude(t *testing.T) {
	lexer := DefaultLexer()
	animationName := sync.OnceValue(func() Node {
		return mustParse(t, "animation-name", ParseOptions{Context: "atrulePrelude", Atrule: "keyframes"})
	})
	number := sync.OnceValue(func() Node {
		return mustParse(t, "123", ParseOptions{Context: "atrulePrelude", Atrule: "unknown"})
	})
	customSyntax := sync.OnceValue(func() *Syntax {
		return mustFork(t, &Config{
			Atrules: map[string]AtruleDefinition{
				"-foo-keyframes": {
					Prelude: "<number>",
				},
			},
		})
	})

	t.Run("should match", func(t *testing.T) {
		match := customSyntax().Lexer.MatchAtrulePrelude("keyframes", animationName())

		assertMatchNoError(t, `MatchAtrulePrelude("keyframes", animation-name)`, match)
	})

	t.Run("vendor prefixes", func(t *testing.T) {
		t.Run("vendor prefix", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtrulePrelude("-webkit-keyframes", animationName())

			assertMatchNoError(t, `MatchAtrulePrelude("-webkit-keyframes", animation-name)`, match)
		})

		t.Run("case insensetive with vendor prefix", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtrulePrelude("KEYFRAMES", animationName())
			assertMatchNoError(t, `MatchAtrulePrelude("KEYFRAMES", animation-name)`, match)

			match = customSyntax().Lexer.MatchAtrulePrelude("-VENDOR-Keyframes", animationName())
			assertMatchNoError(t, `MatchAtrulePrelude("-VENDOR-Keyframes", animation-name)`, match)
		})

		t.Run("should use verdor version first", func(t *testing.T) {
			match := customSyntax().Lexer.MatchAtrulePrelude("-foo-keyframes", number())
			assertMatchNoError(t, `MatchAtrulePrelude("-foo-keyframes", 123)`, match)

			match = customSyntax().Lexer.MatchAtrulePrelude("keyframes", number())
			if match.Matched != nil {
				t.Errorf(`MatchAtrulePrelude("keyframes", 123) matched a value, want nil`)
			}
			assertErrorMessage(t, `MatchAtrulePrelude("keyframes", 123) error`, match.Error, "Mismatch\n  syntax: <keyframes-name>\n   value: 123\n  --------^")
		})
	})

	t.Run("should not be matched to empty value", func(t *testing.T) {
		match := lexer.MatchAtrulePrelude("keyframes", mustParse(t, "", ParseOptions{Context: "atrulePrelude", Positions: true}))

		assertNotMatchedAt(t, `MatchAtrulePrelude("keyframes", "")`, match, 1, 1)
	})

	t.Run("should be positive when no prelude and at-rule has no prelude", func(t *testing.T) {
		match := lexer.MatchAtrulePrelude("font-face", nil)

		if match.Matched != nil || match.Error != nil {
			t.Errorf(`MatchAtrulePrelude("font-face", nil) = %v, %v, want nil, nil`, match.Matched, match.Error)
		}
	})

	t.Run("should not match css wide keywords", func(t *testing.T) {
		for _, keyword := range testCSSWideKeywords() {
			t.Run(keyword, func(t *testing.T) {
				match := lexer.MatchAtrulePrelude("import", mustParse(t, keyword, ParseOptions{Context: "atrulePrelude", Positions: true}))

				assertMismatch(t, `MatchAtrulePrelude("import", `+keyword+`)`, match)
			})
		}
	})

	t.Run("should not be matched to at-rules with no prelude", func(t *testing.T) {
		t.Run("regular name", func(t *testing.T) {
			match := lexer.MatchAtrulePrelude("font-face", animationName())

			if match.Matched != nil {
				t.Errorf(`MatchAtrulePrelude("font-face", animation-name) matched a value, want nil`)
			}
			assertErrorMessage(t, `MatchAtrulePrelude("font-face", animation-name) error`, match.Error, "At-rule `@font-face` should not contain a prelude")
		})

		t.Run("with verdor prefix", func(t *testing.T) {
			match := lexer.MatchAtrulePrelude("-prefix-font-face", animationName())

			if match.Matched != nil {
				t.Errorf(`MatchAtrulePrelude("-prefix-font-face", animation-name) matched a value, want nil`)
			}
			assertErrorMessage(t, `MatchAtrulePrelude("-prefix-font-face", animation-name) error`, match.Error, "At-rule `@-prefix-font-face` should not contain a prelude")
		})
	})

	t.Run("should not fail when prelude is not specified and syntax allows it", func(t *testing.T) {
		nullMatch := &MatchNode{
			Match: []*MatchNode{},
			Syntax: &SyntaxRef{
				Name: "page",
				Kind: "AtrulePrelude",
			},
		}

		// upstream passes '', null and undefined
		for _, test := range []struct {
			name  string
			match func() *MatchResult
		}{
			{"", func() *MatchResult { return lexer.MatchAtrulePreludeString("page", "") }},
			{"null", func() *MatchResult { return lexer.MatchAtrulePrelude("page", nil) }},
			{"undefined", func() *MatchResult { return lexer.MatchAtrulePrelude("page", nil) }},
		} {
			t.Run(test.name, func(t *testing.T) {
				actual := test.match()

				if actual.Error != nil {
					t.Errorf(`MatchAtrulePrelude("page") error = %v, want nil`, actual.Error)
				}
				if !reflect.DeepEqual(actual.Matched, nullMatch) {
					t.Errorf(`MatchAtrulePrelude("page") matched = %+v, want %+v`, actual.Matched, nullMatch)
				}
			})
		}
	})

	forEachAtrulePreludeTest(t, func(t *testing.T, testType, testState, name string, lexer *Lexer, atruleName, value string) {
		switch testType {
		case "valid":
			runDefinitionSyntaxValue(t, testState, name, func(t *testing.T) {
				match := lexer.MatchAtrulePreludeString(atruleName, value)

				if match.Matched == nil {
					t.Errorf("MatchAtrulePreludeString(%q, %q) = nil, want a match: %v", atruleName, value, match.Error)
				}
			})

		case "invalid":
			runDefinitionSyntaxValue(t, testState, name, func(t *testing.T) {
				match := lexer.MatchAtrulePreludeString(atruleName, value)

				if match.Matched != nil {
					t.Errorf("should NOT MATCH to %q", value)
				}
				// upstream: error.name is SyntaxMatchError or SyntaxError
				var matchError *SyntaxMatchError
				var atruleError *AtruleSyntaxError
				if !errors.As(match.Error, &matchError) && !errors.As(match.Error, &atruleError) {
					t.Errorf("MatchAtrulePreludeString(%q, %q) error = %v, want a *SyntaxMatchError or an *AtruleSyntaxError", atruleName, value, match.Error)
				}
			})
		}
	})
}
